-- Permanent channel removal is an auditable lifecycle transition.
-- Prior constraints admitted reconnect, disconnect and provisioning failure,
-- but rejected the actual action used by RemoveChannelCommandService:
-- 'remove_requested'. The failed INSERT rolled back the local purge/archive.
-- Preserve all previously allowed actions and only add the missing one.
ALTER TABLE channel_connection_state_events
    DROP CONSTRAINT channel_connection_state_events_action_chk;

ALTER TABLE channel_connection_state_events
    ADD CONSTRAINT channel_connection_state_events_action_chk
    CHECK (action IN (
        'reconnect_requested',
        'disconnect_requested',
        'provisioning_activation_failed',
        'remove_requested'
    ));
