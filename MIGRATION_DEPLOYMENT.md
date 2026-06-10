# 🚀 Automated Database Migration Deployment

Your backend now has **automatic database migrations** that run on startup!

---

## What Changed

### New Files Created
1. **`internal/database/migrations.go`** - Automatic migration runner
2. **`migrations/001_add_account_lockout.up.sql`** - Account lockout feature migration
3. **`cmd/api/main.go`** - Updated to call migrations on startup

### How It Works
1. App starts and connects to database
2. Creates `schema_migrations` table to track applied migrations
3. Reads all `.up.sql` files from `migrations/` directory
4. Runs only migrations that haven't been applied yet (idempotent)
5. Records each migration in the database
6. Continues with app startup

---

## Deployment Steps

### Step 1: Build New Docker Image
```bash
cd ~/apps/wisdomchurch/infra/prod
docker build -t wisdom-api:v2 -f dockerfile .
```

### Step 2: Deploy to Production
```bash
# Update docker-compose to use new image
# In docker-compose-prod.yml, update the image to wisdom-api:v2

docker-compose -f docker-compose-prod.yml up -d
```

The migrations will run **automatically** when the container starts! 🎉

### Step 3: Verify Migrations Ran
```bash
# Check logs for migration messages
docker logs wisdom_api | grep -i "migration\|account"

# You should see:
# → Running migration: 001_add_account_lockout.up.sql
# ✓ Migration completed: 001_add_account_lockout.up.sql
# All migrations completed successfully
```

### Step 4: Verify Database Changes
Connect to Supabase and verify columns were added:
```sql
SELECT column_name, data_type FROM information_schema.columns 
WHERE table_name = 'users' 
AND column_name IN ('is_locked', 'locked_until', 'failed_login_count', 'last_failed_login_at');
```

Should show:
```
is_locked              | boolean
locked_until           | timestamp
failed_login_count     | integer
last_failed_login_at   | timestamp
```

---

## What Migrations Added

### Account Lockout Feature
Tracks failed login attempts and locks accounts:
- `failed_login_count` - Counter for failed attempts
- `last_failed_login_at` - Timestamp of last failure
- `is_locked` - Account lock status (true/false)
- `locked_until` - When lock expires

### Indexes
- `idx_users_is_locked` - For fast locked account queries
- `idx_users_locked_until` - For checking lock expiration

---

## How to Add Future Migrations

### Create New Migration File
```bash
# File naming: NNN_description.up.sql (NNN = number, increasing)
# Example: 002_add_two_factor_auth.up.sql

# In migrations/002_add_two_factor_auth.up.sql:
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_backup_codes TEXT[];
CREATE INDEX idx_users_totp ON users(totp_enabled);
```

### That's It!
Just create the `.up.sql` file. When you deploy the next version:
1. Build new Docker image
2. Migrations run automatically on startup
3. No manual SQL commands needed ✅

---

## Migration Features

✅ **Automatic** - Runs on app startup, no manual steps
✅ **Idempotent** - Safe to run multiple times
✅ **Tracked** - Records which migrations have been applied
✅ **Ordered** - Runs in alphabetical order (schema.up.sql first)
✅ **Transactional** - Each migration wrapped in transaction (safe rollback)
✅ **Cloud-Ready** - Works with Supabase, RDS, any PostgreSQL

---

## Troubleshooting

### Migration Failed Error
```
❌ Failed to run migrations: ...
```

**Solution:**
1. Check logs for specific SQL error
2. Verify migration file syntax
3. Check if columns already exist (idempotent should handle this)
4. Run migration manually if needed

### Columns Still Don't Exist
```bash
# Check what migrations have run
docker exec wisdom-postgres psql -U postgres -d wisdom_house_dev -c "SELECT * FROM schema_migrations;"

# Or from Supabase console:
SELECT * FROM schema_migrations;
```

### Need to Rollback
Remove the record from `schema_migrations` table (manual process):
```sql
DELETE FROM schema_migrations WHERE name = '001_add_account_lockout.up.sql';
```

---

## Production Checklist

- [ ] Build new Docker image
- [ ] Update docker-compose-prod.yml with new image
- [ ] Deploy: `docker-compose -f docker-compose-prod.yml up -d`
- [ ] Check logs for migration success
- [ ] Verify database columns exist
- [ ] Test OTP login flow
- [ ] Monitor for errors in logs
- [ ] Test failed login attempts
- [ ] Confirm account lockout works

---

## Success! 🎉

Your database migrations are now:
✅ Automatic
✅ Safe
✅ Tracked
✅ Production-ready

No more manual SQL! Deploy with confidence.
