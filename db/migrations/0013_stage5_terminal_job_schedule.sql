-- Terminal jobs have no future schedule. The original queue made this column
-- mandatory for ready jobs; terminal outcomes need an explicit NULL instead.

ALTER TABLE jobs
    ALTER COLUMN next_attempt_at DROP NOT NULL;
