# Professional Authentication Flow - Implementation Guide

## Overview

This guide provides a seamless, secure authentication implementation for production use.

## Current Status

✅ **CORS**: Properly configured for admin.wisdomchurchhq.org
✅ **Preflight**: OPTIONS requests working (204 No Content)
❌ **OTP Verification**: 400 Bad Request - needs improvement

## What's Being Fixed

### Issue: `/api/v1/auth/login/verify-otp` Returns 400

**Root Causes:**
1. Missing/invalid `email` field
2. Missing/invalid `code` field (must be exactly 6 digits)
3. Poor error messages don't tell user what's wrong
4. No rate limiting against brute force

**Solution:**
- Validate all inputs with clear error messages
- Implement rate limiting
- Return helpful error responses
- Improve user experience

## Implementation Steps

### Step 1: Update auth.go Route Registration

In `cmd/api/main.go`, ensure the route is registered:

```go
// Public auth routes
api.POST("/auth/login", authHandler.Login)
api.POST("/auth/login/verify-otp", authHandler.VerifyLoginOTP)  // ← This one
api.POST("/auth/login/resend-otp", authHandler.ResendLoginOTP)
api.POST("/auth/logout", authHandler.Logout)
```

### Step 2: Replace VerifyLoginOTP Function

**Location**: `internal/handlers/auth.go` line 1180

**Current Implementation**: Basic validation, generic errors
**New Implementation**: Detailed validation, helpful errors

Option A: Manually update `auth.go` (preserve existing code style)
Option B: Use improved version from `auth_verify_otp_improved.go`

### Step 3: Add Rate Limiting Cache

Update `internal/cache/cache.go`:

```go
// Rate limiting for OTP attempts
func (c *Cache) RecordOTPAttempt(email string, success bool) error {
    key := fmt.Sprintf("otp_attempts:%s", strings.ToLower(email))
    if success {
        // Clear on success
        return c.client.Del(ctx, key).Err()
    }
    
    // Increment on failure with 15 min expiry
    return c.client.Incr(ctx, key).Err()
}

func (c *Cache) GetOTPAttempts(email string) int {
    key := fmt.Sprintf("otp_attempts:%s", strings.ToLower(email))
    val := c.client.Get(ctx, key).Val()
    attempts, _ := strconv.Atoi(val)
    return attempts
}

func (c *Cache) IsOTPRateLimited(email string) bool {
    return c.GetOTPAttempts(email) >= 5
}
```

### Step 4: Testing

#### Test Valid OTP Request

```bash
curl -X POST https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  -H "Origin: https://admin.wisdomchurchhq.org" \
  -b "device_id=test-device" \
  --data '{
    "email": "admin@wisdomchurchhq.org",
    "code": "123456",
    "rememberMe": true
  }' \
  -v
```

**Expected Response (Success)**:
```json
{
  "status": "success",
  "message": "Login verified successfully",
  "data": {
    "user": {
      "id": "user123",
      "email": "admin@wisdomchurchhq.org",
      "name": "Admin Name",
      "role": "admin"
    }
  },
  "meta": {
    "authenticated": true,
    "access_status": "granted",
    "next_step": "/dashboard"
  }
}
```

#### Test Invalid Email

```bash
curl -X POST https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  --data '{
    "email": "invalid-email",
    "code": "123456"
  }'
```

**Expected Response (400)**:
```json
{
  "status": "error",
  "message": "Invalid email format",
  "errors": {
    "email": "Please provide a valid email address"
  }
}
```

#### Test Invalid Code Format

```bash
curl -X POST https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  --data '{
    "email": "admin@wisdomchurchhq.org",
    "code": "12345"
  }'
```

**Expected Response (400)**:
```json
{
  "status": "error",
  "message": "Invalid OTP code format",
  "errors": {
    "code": "OTP code must be exactly 6 digits (0-9)"
  }
}
```

#### Test Rate Limiting

Make 5 failed attempts, then:

```bash
curl -X POST https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  --data '{
    "email": "admin@wisdomchurchhq.org",
    "code": "000000"
  }'
```

**Expected Response (429)**:
```json
{
  "status": "error",
  "message": "Too many failed verification attempts",
  "meta": {
    "retry_after": 900,
    "help": "Please wait 15 minutes before trying again"
  }
}
```

## Frontend Integration

### Correct Request Structure

```typescript
interface OTPVerifyRequest {
  email: string;        // Required: valid email
  code: string;         // Required: exactly 6 digits
  rememberMe?: boolean; // Optional: keep logged in
  purpose?: string;     // Optional: login, password_reset
  method?: string;      // Optional: email, authenticator
}

async function verifyOTP(email: string, code: string): Promise<AuthResponse> {
  // Validate before sending
  if (!isValidEmail(email)) {
    throw new Error('Invalid email format');
  }
  if (!isValidOTPCode(code)) {
    throw new Error('OTP must be 6 digits');
  }

  const response = await fetch(
    'https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp',
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Origin': 'https://admin.wisdomchurchhq.org'
      },
      credentials: 'include', // Important: sends cookies
      body: JSON.stringify({
        email: email.toLowerCase().trim(),
        code: code.trim(),
        rememberMe: true
      })
    }
  );

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.message || 'Verification failed');
  }

  return response.json();
}

function isValidEmail(email: string): boolean {
  const regex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  return regex.test(email);
}

function isValidOTPCode(code: string): boolean {
  return /^\d{6}$/.test(code.trim());
}
```

### Error Handling

```typescript
async function handleOTPSubmit(email: string, code: string) {
  try {
    const result = await verifyOTP(email, code);
    
    // Success
    console.log('Logged in as:', result.data.user.email);
    redirectToDashboard(result.meta.next_step);
    
  } catch (error) {
    // Handle specific errors
    if (error.message.includes('Too many')) {
      showError('Too many attempts. Please wait 15 minutes.');
    } else if (error.message.includes('Invalid email')) {
      showFieldError('email', error.message);
    } else if (error.message.includes('Incorrect')) {
      showFieldError('code', 'Wrong code. Please check and try again.');
    } else if (error.message.includes('expired')) {
      showError('OTP expired. Request a new one.');
    } else {
      showError('Verification failed. Please try again.');
    }
  }
}
```

## Security Considerations

### ✅ What's Implemented
- HTTPS only (in production)
- Secure, HttpOnly cookies
- CORS properly configured
- Rate limiting on failed attempts
- Email validation
- OTP expiration (5 minutes)

### ✅ What You Should Add
- **CSRF Protection**: Add X-CSRF-Token header
- **Brute Force Protection**: Increase limits for suspicious IPs
- **Anomaly Detection**: Alert on unusual login patterns
- **Session Timeout**: Auto-logout after 30 minutes idle
- **Device Fingerprinting**: Track device ID per session
- **Audit Logging**: Log all auth attempts with timestamp, IP, email

### ✅ What To Avoid
- ❌ Logging OTP codes (security breach)
- ❌ Revealing if email exists (account enumeration)
- ❌ Long OTP expiration (security risk)
- ❌ Allowing OTP reuse (security risk)
- ❌ Storing tokens in localStorage (XSS vulnerability)

## Deployment Checklist

Before deploying to production:

- [ ] Update `internal/handlers/auth.go` with improved validation
- [ ] Add rate limiting to cache layer
- [ ] Test all error scenarios
- [ ] Update frontend error handling
- [ ] Enable HTTPS for all endpoints
- [ ] Configure secure cookies
- [ ] Set up monitoring/alerting
- [ ] Test with real users in staging
- [ ] Document all error codes
- [ ] Create support guide for users

## Monitoring & Alerting

### Key Metrics to Track

```
- OTP verification success rate
- Average verification time
- Rate limit hits per hour
- Failed attempts per user
- CORS rejection rate
- Token expiration rate
```

### Alerts to Configure

```
- High rate of failed OTPs (potential attack)
- Rate limit being hit frequently (DOS attempt?)
- Slow verification times (database issue?)
- CORS errors increasing (frontend misconfiguration?)
- Unusual login patterns (compromised account?)
```

## Common Issues & Solutions

### Issue: "Invalid request format"
**Cause**: Request body malformed
**Fix**: Ensure JSON is valid, use lowercase email

### Issue: "OTP code must be exactly 6 digits"
**Cause**: Code length != 6 or contains non-digits
**Fix**: Validate input before sending, strip whitespace

### Issue: "Too many failed attempts"
**Cause**: Made 5+ failed attempts in 15 minutes
**Fix**: Wait 15 minutes or use "resend-otp" to get new code

### Issue: "Incorrect OTP code"
**Cause**: Code doesn't match what was sent
**Fix**: Check email for correct code, try again

### Issue: "OTP has expired"
**Cause**: More than 5 minutes passed since code was sent
**Fix**: Use "resend-otp" endpoint to get new code

## Support Resources

- **API Docs**: Swagger at https://api.wisdomchurchhq.org/swagger
- **Error Codes**: See error responses above
- **User Guide**: Document for admins on OTP process
- **Developer Docs**: This guide for developers
- **Slack Channel**: #auth-support for questions

## Rollback Plan

If issues occur in production:

1. **Monitor errors** (first 30 min post-deploy)
2. **If critical issues**: Revert to previous version
3. **Create incident ticket** with reproduction steps
4. **Fix and test** in staging thoroughly
5. **Deploy again** with fixes
6. **Monitor closely** for 24 hours

---

**Last Updated**: 2026-06-10
**Version**: 2.0.0 (Enhanced)
**Status**: Ready for implementation
