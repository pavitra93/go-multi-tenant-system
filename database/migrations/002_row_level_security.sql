-- Enable Row Level Security (RLS) for multi-tenant data isolation

-- Enable RLS on all tables
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE location_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE locations ENABLE ROW LEVEL SECURITY;

-- Create a function to get current tenant ID from JWT claims
-- This function will be called by the application to set the current tenant context
CREATE OR REPLACE FUNCTION set_tenant_context(tenant_uuid UUID)
RETURNS void AS $$
BEGIN
    -- Set the current tenant ID in the session
    PERFORM set_config('app.current_tenant_id', tenant_uuid::text, true);
END;
$$ LANGUAGE plpgsql;

-- Create a function to get current tenant ID
CREATE OR REPLACE FUNCTION get_current_tenant_id()
RETURNS UUID AS $$
BEGIN
    RETURN COALESCE(
        current_setting('app.current_tenant_id', true)::UUID,
        '00000000-0000-0000-0000-000000000000'::UUID
    );
END;
$$ LANGUAGE plpgsql;

-- Create a function to check if user is admin
CREATE OR REPLACE FUNCTION is_admin()
RETURNS BOOLEAN AS $$
BEGIN
    RETURN COALESCE(
        current_setting('app.user_role', true) = 'admin',
        false
    );
END;
$$ LANGUAGE plpgsql;

-- Create a function to check if user is tenant owner
CREATE OR REPLACE FUNCTION is_tenant_owner()
RETURNS BOOLEAN AS $$
BEGIN
    RETURN COALESCE(
        current_setting('app.user_role', true) = 'tenant_owner',
        false
    );
END;
$$ LANGUAGE plpgsql;

-- Set user role function
CREATE OR REPLACE FUNCTION set_user_role(role_name TEXT)
RETURNS void AS $$
BEGIN
    PERFORM set_config('app.user_role', role_name, true);
END;
$$ LANGUAGE plpgsql;

-- RLS Policies for tenants table
-- Admins can see all tenants, regular users can only see their own tenant
CREATE POLICY tenant_access_policy ON tenants
    FOR ALL
    TO PUBLIC
    USING (
        is_admin() OR 
        id = get_current_tenant_id()
    );

-- RLS Policies for users table
-- Admins can see all, tenant owners can see their tenant's users, regular users can see their tenant
CREATE POLICY user_access_policy ON users
    FOR ALL
    TO PUBLIC
    USING (
        is_admin() OR 
        tenant_id = get_current_tenant_id()
    );

-- RLS Policies for location_sessions table
-- Users can only see sessions from their own tenant
CREATE POLICY location_session_access_policy ON location_sessions
    FOR ALL
    TO PUBLIC
    USING (
        is_admin() OR 
        tenant_id = get_current_tenant_id()
    );

-- RLS Policies for locations table
-- Users can only see locations from their own tenant
CREATE POLICY location_access_policy ON locations
    FOR ALL
    TO PUBLIC
    USING (
        is_admin() OR 
        tenant_id = get_current_tenant_id()
    );

-- Create a function to automatically set updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create triggers to automatically update updated_at
CREATE TRIGGER update_tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_location_sessions_updated_at
    BEFORE UPDATE ON location_sessions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_locations_updated_at
    BEFORE UPDATE ON locations
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
