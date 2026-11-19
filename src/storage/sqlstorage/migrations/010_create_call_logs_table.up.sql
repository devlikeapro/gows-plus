-- The account's call log (voice / video / group calls), fed by history sync and the app-state call_log collection
CREATE TABLE gows_call_logs
(
    -- Call ID
    id VARCHAR(100) NOT NULL,
    -- Chat the call belongs to: the other party, or the group of a group call
    jid VARCHAR(100) NOT NULL,
    -- Call start, unix seconds
    timestamp BIGINT NOT NULL,
    -- Whether this account placed the call
    from_me BOOLEAN NOT NULL,
    -- Call log data (JSON)
    data TEXT NOT NULL,
    PRIMARY KEY (id)
);

CREATE INDEX gows_call_logs_jid_timestamp ON gows_call_logs (jid, timestamp);
