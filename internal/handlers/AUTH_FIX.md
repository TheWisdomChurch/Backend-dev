# Authentication Flow Fix - Seamless OTP Verification

## Current Issue

**Endpoint**: `POST /api/v1/auth/login/verify-otp`
**Status**: 400 Bad Request
**Problem**: OTP verification failing - likely missing or invalid request body

## Root Causes

1. **Missing/Invalid Request Fields**
   ```json
   // REQUIRED fields
   {
     "email": "user@example.com",    // MUST be valid email
     "code": "123456"                // MUST be exactly 6 digits
   }
   
   // OPTIONAL fields
   {
     "purpose": "login",             // Optional: login, password_reset
     "method": "email",              // Optional: email, authenticator
     "rememberMe": true              // Optional: keep session longer
   }
   ```

2. **Invalid Email Format**
   - Email not provided or malformed
   - Not normalized (should be lowercase)

3. **Invalid OTP Code**
   - Code length != 6 digits
   - Code is empty or missing
   - Code has expired (5 min timeout)

4. **Poor Error Messages**
   - User gets generic "400 Bad Request"
   - No indication of what field is invalid

## The Fix

### 1. Improve Error Messages (auth.go:1180)

```go
func (h *AuthHandler) VerifyLoginOTP(c *gin.Context) {
    var req struct {
        Email      string `json:"email" binding:"required,email"`
        Code       string `json:"code" binding:"required,len=6,numeric"`
        Purpose    string `json:"purpose"`
        Method     string `json:"method"`
        RememberMe bool   `json:"rememberMe"`
    }

    if !validation.BindJSON(c, &req) {
        // Current: Returns generic error
        // Improved: Return specific field errors
        return
    }

    // Normalize and validate email
    req.Email = validation.NormalizeEmail(req.Email)
    
    // Validate code format
    if !regexp.MustCompile(`^\d{6}$`).MatchString(req.Code) {
        utils.ErrorResponse(c, http.StatusBadRequest, "OTP code must be exactly 6 digits")
        return
    }

    // ... rest of verification
}
```

### 2. Enhanced Error Response Format

**Before** (unhelpful):
```json
{
  "error": "Invalid verification response"
}
```

**After** (helpful):
```json
{
  "status": "error",
  "message": "OTP verification failed",
  "errors": {
    "code": "OTP code must be 6 digits",
    "email": "Invalid email format"
  },
  "data": {
    "help": "Check the OTP code from your email and try again",
    "retry_after": 60
  }
}
```

### 3. Add Rate Limiting

```go
// Prevent brute force attacks
const MaxOTPAttempts = 5
const OTPAttemptWindow = 15 * time.Minute

func (h *AuthHandler) VerifyLoginOTP(c *gin.Context) {
    // ... validation ...
    
    // Check rate limit
    attempts := h.cache.GetOTPAttempts(req.Email)
    if attempts >= MaxOTPAttempts {
        utils.ErrorResponse(c, http.StatusTooManyRequests, 
            "Too many failed attempts. Try again in 15 minutes.")
        return
    }
    
    // Verify OTP
    user, authMethod, err := h.service.VerifyLoginMFA(...)
    if err != nil {
        h.cache.IncrementOTPAttempts(req.Email)
        utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
        return
    }
    
    // Success: clear attempts
    h.cache.ClearOTPAttempts(req.Email)
    // ... rest of flow
}
```

### 4. Session Management for Seamless UX

```go
// Issue JWT token with proper expiration
func (h *AuthHandler) issueAuthenticatedSession(c *gin.Context, user *models.User, rememberMe bool, authMethod string) error {
    ttl := h.accessTokenTTL
    if rememberMe {
        ttl = h.rememberMeTTL  // 30 days instead of 24 hours
    }

    token, err := h.generateJWT(user, authMethod, ttl)
    if err != nil {
        return err
    }

    // Set secure cookie
    c.SetCookie(
        authTokenCookieName,
        token,
        int(ttl.Seconds()),
        "/",
        ".wisdomchurchhq.org",  // Allow subdomains
        h.secure,               // HTTPS only in production
        true,                   // HttpOnly - prevent XSS
    )

    return nil
}
```

## Implementation Checklist

- [ ] **Improve validation** - Check all required fields
- [ ] **Better error messages** - Return specific field errors
- [ ] **Add numeric validation** - Code must be 6 digits
- [ ] **Rate limiting** - Prevent brute force (5 attempts / 15 min)
- [ ] **Session management** - Proper JWT with TTL
- [ ] **Cookie security** - Secure, HttpOnly, SameSite
- [ ] **Email normalization** - Lowercase all emails
- [ ] **Logging** - Log all auth attempts (without passwords)

## Frontend Integration

### Correct Request Format

```javascript
// CORRECT ✅
async function verifyOTP(email, code) {
  const response = await fetch('https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Origin': 'https://admin.wisdomchurchhq.org'
    },
    credentials: 'include',  // Send cookies
    body: JSON.stringify({
      email: email.toLowerCase(),  // Normalize!
      code: code.trim(),           // Remove whitespace!
      rememberMe: true             // Optional
    })
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.message || 'OTP verification failed');
  }

  return response.json();
}
```

### Request Validation on Frontend

```javascript
function validateOTPInput(email, code) {
  const errors = {};

  // Validate email
  if (!email || !email.includes('@')) {
    errors.email = 'Please enter a valid email';
  }

  // Validate code
  if (!code || code.length !== 6) {
    errors.code = 'OTP must be 6 digits';
  }
  if (!/^\d+$/.test(code)) {
    errors.code = 'OTP must contain only numbers';
  }

  return { valid: Object.keys(errors).length === 0, errors };
}
```

## API Response Examples

### Success Response
```json
{
  "status": "success",
  "message": "Login verified",
  "data": {
    "user": {
      "id": "user123",
      "email": "user@wisdomchurchhq.org",
      "name": "John Doe",
      "role": "admin"
    }
  },
  "meta": {
    "authenticated": true,
    "access_status": "granted",
    "access_code": null,
    "next_step": "/dashboard"
  }
}
```

### Error Response (Missing Code)
```json
{
  "status": "error",
  "message": "OTP verification failed",
  "errors": {
    "code": "OTP code is required and must be 6 digits"
  }
}
```

### Error Response (Rate Limited)
```json
{
  "status": "error",
  "message": "Too many failed attempts",
  "meta": {
    "retry_after": 900,
    "help": "Please try again in 15 minutes"
  }
}
```

## Security Best Practices

✅ **DO:**
- Validate email format on backend
- Check code length (6 digits)
- Implement rate limiting
- Log auth attempts (not codes!)
- Use HTTPS only
- Set secure, httponly cookies
- Normalize emails to lowercase

❌ **DON'T:**
- Log OTP codes
- Return helpful "user not found" messages (leak account enumeration)
- Accept codes longer than 6 digits
- Use localStorage for tokens
- Send tokens in URL
- Allow code reuse

## Testing the Fix

### Test Valid OTP
```bash
curl -X POST https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  -H "Origin: https://admin.wisdomchurchhq.org" \
  -b "device_id=xxxxx" \
  -d '{
    "email": "user@example.com",
    "code": "123456",
    "rememberMe": true
  }'
```

### Test Invalid OTP
```bash
curl -X POST https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "code": "12345"  // Only 5 digits
  }'
# Should return 400 with clear error message
```

## Deployment Steps

1. **Update auth handler** with improved validation
2. **Add rate limiting** service
3. **Improve error messages** 
4. **Test thoroughly**
5. **Deploy to staging** first
6. **Test with admin users**
7. **Monitor logs** for auth failures
8. **Deploy to production**

## Monitoring & Alerts

Add alerts for:
- High rate of failed OTP attempts (potential attack)
- Slow OTP verification (database issue)
- Missing cookies (session issue)
- CORS rejections (frontend misconfiguration)
