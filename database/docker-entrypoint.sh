#!/bin/bash
set -e

# Wait for PostgreSQL to be ready
echo "Waiting for PostgreSQL to be ready..."
while ! pg_isready -h $DB_HOST -p $DB_PORT -U $DB_USER; do
    sleep 1
done

echo "PostgreSQL is ready!"

# Run database initialization
echo "Running database initialization..."
psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f /docker-entrypoint-initdb.d/init.sql

echo "Database initialization completed!"
