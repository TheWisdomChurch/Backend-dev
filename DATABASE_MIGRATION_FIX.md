# 🚨 CRITICAL: Database Migration Fix

**Issue**: Missing database columns blocking authentication
**Status**: BLOCKER - Users cannot log in
**Fix Time**: 5 minutes
**Risk**: LOW (additive migration, no data loss)

---

## The Problem

```
ERROR: column "is_locked" of relation "users" does not exist (SQLSTATE 42703)
```

Your code expects these columns in the `users` table:
- `is_locked` (boolean)
- `locked_until` (timestamp)
- `failed_login_count` (integer)
- `last_failed_login_at` (timestamp)

But your database doesn't have them yet.

---

## Quick Fix (5 Minutes)

### Step 1: Run the Migration

```bash
# Connect to your database and run:
psql -h <host> -U postgres -d wisdom_house_dev -f migrations/add_account_lockout_to_users.sql

# Or via Docker:
docker exec wisdom-postgres psql -U postgres -d wisdom_house_dev -f /migrations/add_account_lockout_to_users.sql
```

### Step 2: Verify Columns Were Added

```bash
psql -h <host> -U postgres -d wisdom_house_dev -c "\d users"

# Look for these columns in output:
# - failed_login_count (integer)
# - last_failed_login_at (timestamp)
# - is_locked (boolean)
# - locked_until (timestamp)
```

### Step 3: Restart Backend

```bash
# Stop current backend
docker stop wisdom_api

# Start fresh
docker-compose up -d
```

### Step 4: Test Authentication

```bash
# Verify OTP endpoint works
curl -X POST http://localhost:8080/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","code":"123456"}'

# Should return 400 with clear error message (not database error)
```

---

## What Was Added

### 1. Database Migration File
**File**: `migrations/add_account_lockout_to_users.sql`

Adds 4 columns to track failed login attempts:
- `failed_login_count` - counts failed attempts
- `last_failed_login_at` - timestamp of last failure
- `is_locked` - whether account is locked
- `locked_until` - when lock expires

Includes:
- ✅ Indexes for fast queries
- ✅ Constraints for data integrity
- ✅ Verification query

### 2. Enterprise Error Handler
**File**: `internal/apperror/error_handler.go`

Professional error handling with:
- ✅ Standardized error responses
- ✅ Error detection (database, validation, etc.)
- ✅ Helpful metadata per error type
- ✅ Request ID tracking
- ✅ Structured logging

---

## Expected Results After Fix

### Before Fix
```
❌ POST /auth/login/verify-otp → 400 (database error)
❌ Database logs: "column is_locked does not exist"
❌ Users cannot authenticate
```

### After Fix
```
✅ POST /auth/login/verify-otp → 400 with clear message (if code wrong)
✅ Database logs: No schema errors
✅ Users can authenticate successfully
✅ Failed login attempts tracked for security
```

---

## Using the New Error Handler

The error handler is now available for all handlers:

```go
// In your handlers
import "wisdomHouse-backend/internal/apperror"

// Validation error
apperror.RespondErrorWithValidation(c, map[string]string{
    "email": "Email is required",
    "code": "OTP must be 6 digits",
})

// Simple error
apperror.RespondError(c, http.StatusBadRequest, apperror.CodeBadRequest, "Invalid request")

// Error with details
apperror.RespondErrorWithDetails(c, 
    http.StatusBadRequest,
    apperror.CodeValidationError,
    "Validation failed",
    map[string]string{
        "email": "Invalid format",
    },
)

// Success response
apperror.RespondSuccess(c, gin.H{"user": user})
```

---

## Verify in Production

After deploying:

```bash
# 1. Check no database errors in logs
docker logs wisdom_api | grep -i "column.*does not exist"
# Should return nothing

# 2. Check auth success rate
# Monitor: successful login completions increase

# 3. Check error responses are helpful
# Login with wrong OTP code should show:
# "Incorrect OTP code" (not database error)
```

---

## If You Get Stuck

### Migration fails?
```bash
# Check if columns already exist
psql -h <host> -U postgres -d wisdom_house_dev \
  -c "SELECT column_name FROM information_schema.columns WHERE table_name='users';"
```

### Still seeing database errors?
```bash
# Restart database container
docker-compose restart postgres

# Then restart backend
docker-compose restart wisdom_api
```

### Need to rollback?
```bash
# Remove the columns (if needed)
psql -h <host> -U postgres -d wisdom_house_dev \
  -c "ALTER TABLE users DROP COLUMN IF EXISTS is_locked, locked_until, failed_login_count, last_failed_login_at;"
```

---

## Summary

✅ **Database migration** - Adds missing columns
✅ **Error handler** - Professional error responses  
✅ **Account lockout** - Security feature included
✅ **Logging** - Full error context tracking
✅ **5-minute fix** - Deploy and verify quickly

**Your authentication will work professionally after this fix!** 🚀
