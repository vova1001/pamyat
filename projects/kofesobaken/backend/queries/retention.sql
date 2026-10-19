-- Срок хранения персональных данных гостей (политика конфиденциальности, п. 4): 1 год.
-- Платежи не удаляются: в них нет имён и телефонов, а учёт может требовать хранить их дольше.

-- name: DeleteOldBookings :execrows
DELETE FROM bookings WHERE created_at < $1 AND day < $1::date;

-- name: DeleteOldOrders :execrows
DELETE FROM orders WHERE created_at < $1;

-- name: DeleteOldSignups :execrows
DELETE FROM event_signups WHERE created_at < $1;

-- name: DeleteOldNotifications :execrows
DELETE FROM notifications WHERE created_at < $1;

-- name: DeleteOldQuestions :execrows
DELETE FROM questions WHERE created_at < $1;
