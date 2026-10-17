package service

import (
	"testing"
	"time"
)

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"+7 (900) 111-22-33", "+7 (900) 111-22-33", true},
		{"89001112233", "+7 (900) 111-22-33", true},
		{"9001112233", "+7 (900) 111-22-33", true},
		{"7 900 111 22 33", "+7 (900) 111-22-33", true},
		{"+1 900 111 22 33", "", false},
		{"12345", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := NormalizePhone(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("NormalizePhone(%q) = %q, %v; ждали %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestNormalizeTelegram(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"@anna", "@anna", true},
		{"anna_dog", "@anna_dog", true},
		{"https://t.me/anna_dog", "@anna_dog", true},
		{"@an", "@an", false},
		{"@анна", "@анна", false},
	}
	for _, c := range cases {
		got, ok := NormalizeTelegram(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("NormalizeTelegram(%q) = %q, %v", c.in, got, ok)
		}
	}
}

func TestPluralRubDates(t *testing.T) {
	for n, want := range map[int]string{1: "гость", 2: "гостя", 4: "гостя", 5: "гостей", 11: "гостей", 12: "гостей", 21: "гость", 22: "гостя", 111: "гостей"} {
		if got := Plural(n, "гость", "гостя", "гостей"); got != want {
			t.Errorf("Plural(%d) = %s, ждали %s", n, got, want)
		}
	}
	if got := Rub(1_250_00); got != "1 250 ₽" {
		t.Errorf("Rub = %q", got)
	}
	if got := Rub(500_00); got != "500 ₽" {
		t.Errorf("Rub = %q", got)
	}
	if got := HumanDate(time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)); got != "Сб, 17 октября" {
		t.Errorf("HumanDate = %q", got)
	}
	tm, ok := ParseClock("09:05")
	if !ok || Clock(tm) != "09:05" || clockMinutes(tm) != 9*60+5 {
		t.Errorf("ParseClock/Clock: %v %v", ok, Clock(tm))
	}
	if _, ok := ParseClock("25:00"); ok {
		t.Error("25:00 — не время")
	}
	if _, ok := ParseDate("2026-02-30"); ok {
		t.Error("30 февраля — не дата")
	}
	if itoa(0) != "0" || itoa(-12) != "-12" || itoa(305) != "305" {
		t.Error("itoa")
	}
}
