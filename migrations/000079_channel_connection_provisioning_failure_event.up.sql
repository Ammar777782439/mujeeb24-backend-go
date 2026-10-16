-- OAuth may complete successfully while local activation is rejected (for
-- example because the business has reached its subscribed channel capacity).
-- Preserve that distinction in the immutable lifecycle audit instead of
-- recording it as a merchant-requested reconnect.
ALTER TABLE channel_connection_state_events
    DROP CONSTRAINT channel_connection_state_events_action_chk;

ALTER TABLE channel_connection_state_events
    ADD CONSTRAINT channel_connection_state_events_action_chk
    CHECK (action IN (
        'reconnect_requested',
        'disconnect_requested',
        'provisioning_activation_failed'
    ));
