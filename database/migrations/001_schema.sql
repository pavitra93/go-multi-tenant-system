-- =====================================================
-- MULTI-TENANT LOCATION TRACKING SYSTEM
-- Database Schema - Interview Ready
-- =====================================================

-- =====================================================
-- 1. TENANTS TABLE
-- =====================================================
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) NOT NULL UNIQUE,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- =====================================================
-- 2. ADMINS TABLE (Platform Administrators)
-- =====================================================
CREATE TABLE admins (
    cognito_id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP WITH TIME ZONE
);

-- =====================================================
-- 3. USERS TABLE (Tenant Users)
-- =====================================================
CREATE TYPE user_role AS ENUM ('tenant_owner', 'user');

CREATE TABLE users (
    cognito_id VARCHAR(255) PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role user_role NOT NULL DEFAULT 'user',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP WITH TIME ZONE
);

-- =====================================================
-- 4. LOCATION_SESSIONS TABLE
-- =====================================================
CREATE TYPE session_status AS ENUM ('active', 'completed', 'expired');

CREATE TABLE location_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    cognito_user_id VARCHAR(255) NOT NULL REFERENCES users(cognito_id) ON DELETE CASCADE,
    status session_status NOT NULL DEFAULT 'active',
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMP WITH TIME ZONE,
    duration INTEGER NOT NULL DEFAULT 600, -- 10 minutes in seconds
    device_info TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- =====================================================
-- 5. LOCATIONS TABLE
-- =====================================================
CREATE TABLE locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_id UUID NOT NULL REFERENCES location_sessions(id) ON DELETE CASCADE,
    cognito_user_id VARCHAR(255) NOT NULL REFERENCES users(cognito_id) ON DELETE CASCADE,
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude >= -90 AND latitude <= 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude >= -180 AND longitude <= 180),
    accuracy DOUBLE PRECISION,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- =====================================================
-- ROW LEVEL SECURITY (RLS) - Tenant Isolation
-- =====================================================

-- Enable RLS on all tables
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE location_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE locations ENABLE ROW LEVEL SECURITY;

-- RLS Policies - Users can only access their own tenant's data
CREATE POLICY tenant_isolation_policy ON tenants
    USING (id = current_setting('app.current_tenant_id', TRUE)::UUID);

CREATE POLICY user_isolation_policy ON users
    USING (tenant_id = current_setting('app.current_tenant_id', TRUE)::UUID);

CREATE POLICY session_isolation_policy ON location_sessions
    USING (tenant_id = current_setting('app.current_tenant_id', TRUE)::UUID);

CREATE POLICY location_isolation_policy ON locations
    USING (tenant_id = current_setting('app.current_tenant_id', TRUE)::UUID);

-- =====================================================
-- HELPER FUNCTIONS FOR RLS
-- =====================================================

-- Function to set tenant context (called by middleware)
CREATE OR REPLACE FUNCTION set_tenant_context(tenant_uuid UUID)
RETURNS void AS $$
BEGIN
    PERFORM set_config('app.current_tenant_id', tenant_uuid::text, true);
END;
$$ LANGUAGE plpgsql;

-- Function to set user role context
CREATE OR REPLACE FUNCTION set_user_role(role_name TEXT)
RETURNS void AS $$
BEGIN
    PERFORM set_config('app.current_user_role', role_name, true);
END;
$$ LANGUAGE plpgsql;