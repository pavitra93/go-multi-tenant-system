-- Migration: Minimal Users Table
-- This migration restructures the users table to eliminate redundancy
-- Username, role, and email are now stored only in Cognito and accessed via JWT

-- Step 1: Create backup of existing users table
CREATE TABLE IF NOT EXISTS users_backup AS SELECT * FROM users;

-- Step 2: Add cognito_user_id columns to dependent tables
ALTER TABLE location_sessions 
    ADD COLUMN IF NOT EXISTS cognito_user_id VARCHAR(255);

ALTER TABLE locations 
    ADD COLUMN IF NOT EXISTS cognito_user_id VARCHAR(255);

-- Step 3: Migrate data from user_id to cognito_user_id
UPDATE location_sessions ls
SET cognito_user_id = u.cognito_id
FROM users u
WHERE ls.user_id = u.id;

UPDATE locations l
SET cognito_user_id = u.cognito_id
FROM users u
WHERE l.user_id = u.id;

-- Step 4: Add last_login_at column to users
ALTER TABLE users 
    ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP WITH TIME ZONE;

-- Step 5: Drop old foreign key constraints
ALTER TABLE location_sessions 
    DROP CONSTRAINT IF EXISTS location_sessions_user_id_fkey;

ALTER TABLE locations 
    DROP CONSTRAINT IF EXISTS locations_user_id_fkey;

-- Step 6: Make cognito_user_id NOT NULL (after data migration)
ALTER TABLE location_sessions 
    ALTER COLUMN cognito_user_id SET NOT NULL;

ALTER TABLE locations 
    ALTER COLUMN cognito_user_id SET NOT NULL;

-- Step 7: Add indexes on new columns
CREATE INDEX IF NOT EXISTS idx_location_sessions_cognito_user_id 
    ON location_sessions(cognito_user_id);

CREATE INDEX IF NOT EXISTS idx_locations_cognito_user_id 
    ON locations(cognito_user_id);

CREATE INDEX IF NOT EXISTS idx_users_last_login_at 
    ON users(last_login_at);

-- Step 8: Add new foreign key constraints to minimal users table
ALTER TABLE location_sessions 
    ADD CONSTRAINT fk_location_sessions_cognito_user 
    FOREIGN KEY (cognito_user_id) 
    REFERENCES users(cognito_id) 
    ON DELETE CASCADE;

ALTER TABLE locations 
    ADD CONSTRAINT fk_locations_cognito_user 
    FOREIGN KEY (cognito_user_id) 
    REFERENCES users(cognito_id) 
    ON DELETE CASCADE;

-- Step 9: Drop old columns from users table (redundant with Cognito)
ALTER TABLE users 
    DROP COLUMN IF EXISTS id,
    DROP COLUMN IF EXISTS username,
    DROP COLUMN IF EXISTS role,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS deleted_at;

-- Step 10: Make cognito_id the primary key
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_pkey;
ALTER TABLE users ADD PRIMARY KEY (cognito_id);

-- Step 11: Drop old user_id columns (now using cognito_user_id)
ALTER TABLE location_sessions DROP COLUMN IF EXISTS user_id;
ALTER TABLE locations DROP COLUMN IF EXISTS user_id;

-- Step 12: Update tenants relationship (remove Users array FK since it used user.id)
-- The relationship is now maintained through cognito_id

-- Verification queries (commented out - uncomment to verify)
-- SELECT COUNT(*) FROM users;
-- SELECT COUNT(*) FROM location_sessions WHERE cognito_user_id IS NULL;
-- SELECT COUNT(*) FROM locations WHERE cognito_user_id IS NULL;

COMMENT ON TABLE users IS 'Minimal user table - most user data (username, email, role) stored in Cognito and accessed via JWT claims';
COMMENT ON COLUMN users.cognito_id IS 'Cognito user sub - primary key and source of truth';
COMMENT ON COLUMN users.tenant_id IS 'Multi-tenancy - which tenant this user belongs to';
COMMENT ON COLUMN users.created_at IS 'Analytics - when user registered';
COMMENT ON COLUMN users.last_login_at IS 'Activity tracking - last login timestamp';
