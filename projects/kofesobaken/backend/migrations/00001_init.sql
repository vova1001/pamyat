-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE admins (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    admin_id   BIGINT NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- Меню: деньги в копейках.
CREATE TABLE menu_items (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price_kop   BIGINT NOT NULL CHECK (price_kop >= 0),
    category    TEXT NOT NULL CHECK (category IN ('black', 'milk', 'other')),
    photo       TEXT NOT NULL DEFAULT '',
    visible     BOOLEAN NOT NULL DEFAULT TRUE,
    signature   BOOLEAN NOT NULL DEFAULT FALSE,
    sort        INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX menu_items_sort_idx ON menu_items (sort, id);

-- Заказ с собой / доставка.
CREATE TABLE orders (
    id         BIGSERIAL PRIMARY KEY,
    mode       TEXT NOT NULL CHECK (mode IN ('pickup', 'delivery')),
    phone      TEXT NOT NULL,
    address    TEXT NOT NULL DEFAULT '',
    total_kop  BIGINT NOT NULL CHECK (total_kop > 0),
    delivery_kop BIGINT NOT NULL DEFAULT 0,
    status     TEXT NOT NULL CHECK (status IN ('awaiting_payment', 'new', 'done', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at    TIMESTAMPTZ
);
CREATE INDEX orders_status_created_idx ON orders (status, created_at DESC);

CREATE TABLE order_items (
    id           BIGSERIAL PRIMARY KEY,
    order_id     BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    menu_item_id BIGINT REFERENCES menu_items(id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    price_kop    BIGINT NOT NULL,
    qty          INT NOT NULL CHECK (qty > 0)
);
CREATE INDEX order_items_order_idx ON order_items (order_id);

-- Бронь столика с предоплатой.
CREATE TABLE bookings (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    phone       TEXT NOT NULL,
    day         DATE NOT NULL,
    at          TIME NOT NULL,
    guests      INT NOT NULL CHECK (guests BETWEEN 1 AND 10),
    with_dog    BOOLEAN NOT NULL DEFAULT FALSE,
    prepay_kop  BIGINT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('awaiting_payment', 'new', 'confirmed', 'cancelled')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at     TIMESTAMPTZ
);
CREATE INDEX bookings_status_created_idx ON bookings (status, created_at DESC);
CREATE INDEX bookings_day_idx ON bookings (day);

-- Мероприятия и записи.
CREATE TABLE events (
    id             BIGSERIAL PRIMARY KEY,
    title          TEXT NOT NULL,
    kind           TEXT NOT NULL DEFAULT 'встреча',
    day            DATE NOT NULL,
    at             TIME NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    photo          TEXT NOT NULL DEFAULT '',
    seats          INT NOT NULL CHECK (seats BETWEEN 1 AND 500),
    price_kop      BIGINT NOT NULL DEFAULT 0 CHECK (price_kop >= 0),
    dogs_allowed   BOOLEAN NOT NULL DEFAULT TRUE,
    signup_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    status         TEXT NOT NULL CHECK (status IN ('draft', 'published')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX events_status_day_idx ON events (status, day);

CREATE TABLE event_signups (
    id          BIGSERIAL PRIMARY KEY,
    event_id    BIGINT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    via         TEXT NOT NULL CHECK (via IN ('phone', 'tg')),
    contact     TEXT NOT NULL,
    people      INT NOT NULL CHECK (people BETWEEN 1 AND 6),
    with_dog    BOOLEAN NOT NULL DEFAULT FALSE,
    amount_kop  BIGINT NOT NULL DEFAULT 0,
    status      TEXT NOT NULL CHECK (status IN ('awaiting_payment', 'confirmed', 'cancelled')),
    hold_until  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at     TIMESTAMPTZ
);
CREATE INDEX event_signups_event_idx ON event_signups (event_id, status);

-- Платежи: один на бронь / заказ / запись. Статус меняется только по уведомлению платёжной системы.
CREATE TABLE payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind                TEXT NOT NULL CHECK (kind IN ('booking', 'order', 'signup')),
    target_id           BIGINT NOT NULL,
    amount_kop          BIGINT NOT NULL CHECK (amount_kop > 0),
    description         TEXT NOT NULL,
    provider            TEXT NOT NULL,
    provider_payment_id TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL CHECK (status IN ('pending', 'succeeded', 'canceled', 'refunded')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX payments_target_idx ON payments (kind, target_id);

-- Настройки и тексты сайта: ключ → JSON.
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Журнал уведомлений (почта, Telegram): что и куда отправлено.
CREATE TABLE notifications (
    id         BIGSERIAL PRIMARY KEY,
    channel    TEXT NOT NULL CHECK (channel IN ('email', 'telegram')),
    kind       TEXT NOT NULL,
    recipient  TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('sent', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX notifications_created_idx ON notifications (created_at DESC);

-- +goose Down
DROP TABLE notifications;
DROP TABLE settings;
DROP TABLE payments;
DROP TABLE event_signups;
DROP TABLE events;
DROP TABLE bookings;
DROP TABLE order_items;
DROP TABLE orders;
DROP TABLE menu_items;
DROP TABLE sessions;
DROP TABLE admins;
