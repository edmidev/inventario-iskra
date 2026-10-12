-- +goose Up
-- rotated_at distinguishes a token revoked by rotation (eligible for the
-- reuse grace window) from one revoked by logout or a security event.
-- replaced_by links a rotated token to its successor.
ALTER TABLE refresh_tokens
    ADD COLUMN rotated_at  TIMESTAMPTZ,
    ADD COLUMN replaced_by UUID REFERENCES refresh_tokens (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE refresh_tokens
    DROP COLUMN replaced_by,
    DROP COLUMN rotated_at;
