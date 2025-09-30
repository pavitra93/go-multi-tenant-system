-- =====================================================
-- COMPLETE DATABASE SCHEMA
-- Multi-Tenant Location Tracking System
-- =====================================================

-- =====================================================
-- 1. TENANTS TABLE
-- =====================================================
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add comments
COMMENT ON TABLE tenants IS 'Tenant organizations using the system';
COMMENT ON COLUMN tenants.id IS 'Unique tenant identifier';
COMMENT ON COLUMN tenants.name IS 'Tenant organization name';

-- =====================================================
-- 2. USERS TABLE
-- =====================================================
CREATE TABLE IF NOT EXISTS users (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    cognito_id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP WITH TIME ZONE,
    
    -- Constraints
    CONSTRAINT users_cognito_id_key UNIQUE (cognito_id)
);

-- Add comments
COMMENT ON TABLE users IS 'User accounts linked to Cognito and tenants';
COMMENT ON COLUMN users.cognito_id IS 'AWS Cognito user ID (primary key)';
COMMENT ON COLUMN users.tenant_id IS 'Tenant this user belongs to';
COMMENT ON COLUMN users.last_login_at IS 'Last successful login timestamp';

-- =====================================================
-- 3. LOCATION_SESSIONS TABLE
-- =====================================================
CREATE TABLE IF NOT EXISTS location_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    cognito_user_id VARCHAR(255) NOT NULL REFERENCES users(cognito_id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMP WITH TIME ZONE,
    duration INTEGER NOT NULL DEFAULT 600, -- in seconds
    device_info TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Constraints
    CONSTRAINT location_sessions_status_check CHECK (status IN ('active', 'completed', 'expired'))
);

-- Add comments
COMMENT ON TABLE location_sessions IS 'Location tracking sessions (10-minute windows)';
COMMENT ON COLUMN location_sessions.duration IS 'Session duration in seconds (default: 600 = 10 minutes)';
COMMENT ON COLUMN location_sessions.status IS 'Session status: active, completed, or expired';

-- =====================================================
-- 4. LOCATIONS TABLE
-- =====================================================
CREATE TABLE IF NOT EXISTS locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_id UUID NOT NULL REFERENCES location_sessions(id) ON DELETE CASCADE,
    cognito_user_id VARCHAR(255) NOT NULL REFERENCES users(cognito_id) ON DELETE CASCADE,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    accuracy DOUBLE PRECISION,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Constraints
    CONSTRAINT locations_latitude_check CHECK (latitude >= -90 AND latitude <= 90),
    CONSTRAINT locations_longitude_check CHECK (longitude >= -180 AND longitude <= 180)
);

-- Add comments
COMMENT ON TABLE locations IS 'GPS location points submitted by users during sessions';
COMMENT ON COLUMN locations.latitude IS 'Latitude coordinate (-90 to 90)';
COMMENT ON COLUMN locations.longitude IS 'Longitude coordinate (-180 to 180)';
COMMENT ON COLUMN locations.accuracy IS 'GPS accuracy in meters';
COMMENT ON COLUMN locations.timestamp IS 'When the location was recorded (client-side)';

-- =====================================================
-- ROW LEVEL SECURITY (RLS)
-- =====================================================

-- Enable RLS on all tables
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE location_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE locations ENABLE ROW LEVEL SECURITY;

-- RLS Policies for Tenants
CREATE POLICY tenant_isolation_policy ON tenants
    USING (id = current_setting('app.current_tenant_id', TRUE)::UUID);

-- RLS Policies for Users
CREATE POLICY user_isolation_policy ON users
    USING (tenant_id = current_setting('app.current_tenant_id', TRUE)::UUID);

-- RLS Policies for Location Sessions
CREATE POLICY session_isolation_policy ON location_sessions
    USING (tenant_id = current_setting('app.current_tenant_id', TRUE)::UUID);

-- RLS Policies for Locations
CREATE POLICY location_isolation_policy ON locations
    USING (tenant_id = current_setting('app.current_tenant_id', TRUE)::UUID);

-- =====================================================
-- INITIAL SETUP COMPLETE
-- =====================================================

