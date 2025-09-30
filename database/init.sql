-- Database initialization script
-- This script sets up the database for the multi-tenant system

-- Create database if it doesn't exist
-- Note: This needs to be run as a superuser
-- CREATE DATABASE multi_tenant_db;

-- Connect to the database
\c multi_tenant_db;

-- Run migrations in order
\i migrations/001_initial_schema.sql
\i migrations/002_row_level_security.sql
\i migrations/003_sample_data.sql

-- Create a function to check database health
CREATE OR REPLACE FUNCTION check_database_health()
RETURNS TABLE (
    table_name TEXT,
    record_count BIGINT,
    last_updated TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        'tenants'::TEXT,
        COUNT(*)::BIGINT,
        MAX(updated_at)
    FROM tenants
    UNION ALL
    SELECT 
        'users'::TEXT,
        COUNT(*)::BIGINT,
        MAX(updated_at)
    FROM users
    UNION ALL
    SELECT 
        'location_sessions'::TEXT,
        COUNT(*)::BIGINT,
        MAX(updated_at)
    FROM location_sessions
    UNION ALL
    SELECT 
        'locations'::TEXT,
        COUNT(*)::BIGINT,
        MAX(updated_at)
    FROM locations;
END;
$$ LANGUAGE plpgsql;

-- Grant necessary permissions
GRANT USAGE ON SCHEMA public TO postgres;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO postgres;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO postgres;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO postgres;

-- Create a view for tenant statistics
CREATE OR REPLACE VIEW tenant_stats AS
SELECT 
    t.id,
    t.name,
    t.domain,
    t.is_active,
    COUNT(DISTINCT u.id) as user_count,
    COUNT(DISTINCT ls.id) as session_count,
    COUNT(DISTINCT l.id) as location_count,
    MAX(l.timestamp) as last_location_update
FROM tenants t
LEFT JOIN users u ON t.id = u.tenant_id AND u.deleted_at IS NULL
LEFT JOIN location_sessions ls ON t.id = ls.tenant_id AND ls.deleted_at IS NULL
LEFT JOIN locations l ON t.id = l.tenant_id AND l.deleted_at IS NULL
WHERE t.deleted_at IS NULL
GROUP BY t.id, t.name, t.domain, t.is_active;

-- Create a view for user activity
CREATE OR REPLACE VIEW user_activity AS
SELECT 
    u.id,
    u.username,
    u.email,
    u.tenant_id,
    t.name as tenant_name,
    COUNT(DISTINCT ls.id) as total_sessions,
    COUNT(DISTINCT l.id) as total_locations,
    MAX(ls.started_at) as last_session_start,
    MAX(l.timestamp) as last_location_update
FROM users u
JOIN tenants t ON u.tenant_id = t.id
LEFT JOIN location_sessions ls ON u.id = ls.user_id AND ls.deleted_at IS NULL
LEFT JOIN locations l ON u.id = l.user_id AND l.deleted_at IS NULL
WHERE u.deleted_at IS NULL AND t.deleted_at IS NULL
GROUP BY u.id, u.username, u.email, u.tenant_id, t.name;
