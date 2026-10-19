package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"kofesobaken/backend/internal/payments"
	"kofesobaken/backend/internal/store"
)

// Фоновая проверка платежей: гость оплатил, но не вернулся на сайт, а уведомление платёжной системы
// не пришло (не настроено, сервер спал). Тогда статус спрашиваем сами — тем же путём, что и уведомление.
const (
	SweepFresh = 2 * time.Hour  // платежи моложе — проверяем каждую минуту
	SweepMax   = 24 * time.Hour // моложе — раз в 10 минут; старше и всё ещё не оплачены — закрываем
)

// closeUnpaid закрывает то, за что не заплатили. Уже оплаченное или закрытое не трогает.
func closeUnpaid(ctx context.Context, q *store.Queries, p store.Payment) error {
	switch p.Kind {
	case "booking":
		return q.CloseUnpaidBooking(ctx, p.TargetID)
	case "order":
		return q.CloseUnpaidOrder(ctx, p.TargetID)
	case "signup":
		return q.CloseUnpaidSignup(ctx, p.TargetID)
	}
	return nil
}

// SweepPayments — один проход. Всегда — платежи моложе SweepFresh; full — ещё и старше (до SweepMax проверяем,
// старше — закрываем как просроченные). Возвращает, сколько платежей изменило статус.
func (s *Service) SweepPayments(ctx context.Context, full bool) (int, error) {
	now := s.now()
	after, before := now.Add(-SweepFresh), now.Add(time.Minute)
	if full {
		after = now.Add(-RetentionPeriod)
	}
	list, err := s.q.ListPendingPayments(ctx, store.ListPendingPaymentsParams{After: after, Before: before})
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, p := range list {
		if ctx.Err() != nil {
			return changed, ctx.Err()
		}
		if s.sweepOne(ctx, p, now.Sub(p.CreatedAt) > SweepMax) {
			changed++
		}
	}
	return changed, nil
}

// sweepOne проверяет один платёж; expired — ему больше суток: не оплачен — закрываем.
func (s *Service) sweepOne(ctx context.Context, p store.Payment, expired bool) bool {
	cancelEv := payments.Event{Type: payments.EventCanceled, PaymentID: p.ID, ProviderPaymentID: p.ProviderPaymentID}
	ev, err := payments.Event{}, payments.ErrIgnored
	if ch, ok := s.pay.(payments.Checker); ok && p.ProviderPaymentID != "" {
		cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		ev, err = ch.Check(cctx, p.ProviderPaymentID)
		cancel()
	}
	switch {
	case err == nil && ev.PaymentID == p.ID:
		// завершён в платёжной системе (оплачен или отменён/просрочен там) — применяем как уведомление
	case err == nil: // ответ о чужом платеже — не применяем
		slog.Warn("проверка платежа: чужой платёж", "payment_id", p.ID)
		if !expired {
			return false
		}
		ev = cancelEv
	case errors.Is(err, payments.ErrIgnored) || errors.Is(err, payments.ErrBadSignature):
		// ещё не оплачен (или платежа нет в магазине — например, сменили тестовый магазин на боевой)
		if !expired {
			return false
		}
		ev = cancelEv
	default: // сеть, ЮKassa недоступна — попробуем в следующий раз
		slog.Warn("проверка платежа", "payment_id", p.ID, "err", err)
		return false
	}
	if err := s.HandlePaymentEvent(ctx, ev); err != nil {
		slog.Warn("проверка платежа: не применено", "payment_id", p.ID, "err", err)
		return false
	}
	slog.Info("проверка платежа: статус обновлён", "payment_id", p.ID, "event", ev.Type, "closed_by_site", ev == cancelEv)
	return true
}

// RunPaymentSweep: каждую минуту — свежие платежи, каждые 10 минут — все до суток и закрытие просроченных.
// Работает и при настроенных уведомлениях: повторное применение ничего не меняет.
func (s *Service) RunPaymentSweep(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for i := 0; ; i++ {
		if n, err := s.SweepPayments(ctx, i%10 == 0); err != nil && ctx.Err() == nil {
			slog.Error("проверка платежей", "err", err)
		} else if n > 0 {
			slog.Info("проверка платежей: обновлено", "платежей", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
