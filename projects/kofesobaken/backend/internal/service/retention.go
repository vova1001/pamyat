package service

import (
	"context"
	"log/slog"
	"time"
)

// RetentionPeriod — сколько храним данные гостей (брони, заказы, записи, вопросы, журнал уведомлений).
const RetentionPeriod = 365 * 24 * time.Hour

// PurgeOld удаляет данные гостей старше срока хранения. Возвращает, сколько записей удалено.
func (s *Service) PurgeOld(ctx context.Context) (int64, error) {
	cutoff := s.now().Add(-RetentionPeriod)
	var total int64
	for _, del := range []func(context.Context, time.Time) (int64, error){
		s.q.DeleteOldBookings, s.q.DeleteOldOrders, s.q.DeleteOldSignups, s.q.DeleteOldNotifications, s.q.DeleteOldQuestions,
	} {
		n, err := del(ctx, cutoff)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// RunRetention чистит старые данные сразу и затем раз в сутки, пока не отменён ctx.
func (s *Service) RunRetention(ctx context.Context) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		if n, err := s.PurgeOld(ctx); err != nil {
			slog.Error("удаление старых данных", "err", err)
		} else if n > 0 {
			slog.Info("удалены данные старше срока хранения", "записей", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
