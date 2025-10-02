-- =====================================================
-- PERFORMANCE INDEXES
-- Optimized for location tracking workload
-- =====================================================

-- =====================================================
-- TENANTS TABLE INDEXES
-- =====================================================
CREATE INDEX idx_tenants_domain ON tenants(domain);
CREATE INDEX idx_tenants_name ON tenants(name);
CREATE INDEX idx_tenants_created_at ON tenants(created_at);

-- =====================================================
-- ADMINS TABLE INDEXES
-- =====================================================
CREATE INDEX idx_admins_cognito_id ON admins(cognito_id);
CREATE INDEX idx_admins_last_login_at ON admins(last_login_at);

-- =====================================================
-- USERS TABLE INDEXES
-- =====================================================
CREATE INDEX idx_users_tenant_id ON users(tenant_id);
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_last_login_at ON users(last_login_at);

-- =====================================================
-- LOCATION_SESSIONS TABLE INDEXES
-- =====================================================
-- Critical for session validation (most frequent query)
CREATE INDEX idx_location_sessions_lookup ON location_sessions(id, cognito_user_id, tenant_id, status);

-- For checking active sessions by user
CREATE INDEX idx_location_sessions_active ON location_sessions(cognito_user_id, status);

-- For tenant-scoped queries
CREATE INDEX idx_location_sessions_tenant ON location_sessions(tenant_id, status);

-- For time-based queries
CREATE INDEX idx_location_sessions_started_at ON location_sessions(started_at);

-- =====================================================
-- LOCATIONS TABLE INDEXES
-- =====================================================
-- Critical for location inserts (FK validation)
CREATE INDEX idx_locations_session_id ON locations(session_id);
CREATE INDEX idx_locations_tenant_id ON locations(tenant_id);

-- For session location queries
CREATE INDEX idx_locations_session_tenant ON locations(session_id, tenant_id);

-- For user location history
CREATE INDEX idx_locations_user_id ON locations(cognito_user_id);

-- For time-based queries and ordering
CREATE INDEX idx_locations_timestamp ON locations(timestamp DESC);

-- For analytics queries
CREATE INDEX idx_locations_user_session ON locations(cognito_user_id, session_id);