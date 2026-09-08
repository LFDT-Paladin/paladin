#!/bin/bash

count=0

until [ $count -ge 3 ]; do
  pg_isready -U postgres && count=$((count + 1))
  echo "Waiting for postgres to be ready..."
  sleep 1
done

# enables pg_stat_statements views for query database performance at runtime
psql -U postgres -d postgres -c "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;" \
  || echo "Warning: could not create pg_stat_statements extension"