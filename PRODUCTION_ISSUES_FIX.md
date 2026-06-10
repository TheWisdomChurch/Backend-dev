# Production Issues - Professional Fix

## Critical Issues in Production

### Issue 1: 429 Too Many Requests on Auth Refresh
**Error**: `GET /api/v1/auth/me 429 (Too Many Requests)`
**Impact**: Users cannot refresh authentication, forced logouts
**Severity**: 🔴 CRITICAL

### Issue 2: 400 Bad Request on OTP Verification
**Error**: `POST /api/v1/auth/login/verify-otp 400 (Bad Request)`
**Impact**: Users cannot complete login
**Severity**: 🔴 CRITICAL

---

## Root Cause Analysis

### Rate Limiting (429)
The backend is aggressively rate limiting the `/auth/me` endpoint:
- **Current**: Likely too strict (blocking legitimate requests)
- **Cause**: Too many refresh calls from frontend without backoff
- **Effect**: Users locked out after multiple requests

### OTP Verification (400)
The OTP endpoint is rejecting requests:
- **Missing validation**: Request likely missing required fields
- **Poor error messages**: Users don't know what's wrong
- **Frontend issue**: Not sending request correctly

---

## The Fix

### Part 1: Backend - Rate Limiting Configuration

**File**: `internal/middleware/rate_limit.go` (create if doesn't exist)

```go
package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimitConfig defines limits per endpoint
var RateLimitConfig = map[string]RateLimit{
	// Auth endpoints - generous limits for user experience
	"/api/v1/auth/me": {
		RequestsPerSecond: 10,     // 10 req/sec per user
		BurstSize:        50,      // Allow bursts of 50
		Window:           1 * time.Second,
	},
	"/api/v1/auth/login/verify-otp": {
		RequestsPerSecond: 5,      // 5 attempts per second
		BurstSize:        10,      // Max 10 in quick succession
		Window:           1 * time.Second,
	},
	"/api/v1/auth/login": {
		RequestsPerSecond: 3,      // 3 logins per second
		BurstSize:        5,       // Burst up to 5
		Window:           1 * time.Second,
	},
}

type RateLimit struct {
	RequestsPerSecond float64
	BurstSize        int
	Window           time.Duration
}

// RateLimitMiddleware returns per-user rate limiting middleware
func RateLimitMiddleware(endpoint string) gin.HandlerFunc {
	limiters := make(map[string]*rate.Limiter)

	config, exists := RateLimitConfig[endpoint]
	if !exists {
		// Default: 100 req/sec if not configured
		config = RateLimit{
			RequestsPerSecond: 100,
			BurstSize:        200,
			Window:           1 * time.Second,
		}
	}

	return func(c *gin.Context) {
		// Use user ID or IP for rate limiting
		identifier := getUserIdentifier(c)

		// Get or create limiter for this user
		if _, exists := limiters[identifier]; !exists {
			limiters[identifier] = rate.NewLimiter(
				rate.Limit(config.RequestsPerSecond),
				config.BurstSize,
			)
		}

		if !limiters[identifier].Allow() {
			// Return retry-after header
			c.Header("Retry-After", "1")
			c.JSON(http.StatusTooManyRequests, gin.H{
				"status":  "error",
				"message": "Rate limit exceeded",
				"meta": gin.H{
					"retry_after": 1,
					"help":        "Please wait 1 second before trying again",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func getUserIdentifier(c *gin.Context) string {
	// Prefer user ID from JWT
	if userID, exists := c.Get("user_id"); exists {
		return fmt.Sprintf("user:%v", userID)
	}
	
	// Fall back to IP address
	return fmt.Sprintf("ip:%s", c.ClientIP())
}
```

### Part 2: Backend - Better Error Messages

**File**: `internal/handlers/auth.go` - Update VerifyLoginOTP

```go
func (h *AuthHandler) VerifyLoginOTP(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required"`
		Code       string `json:"code" binding:"required"`
		RememberMe bool   `json:"rememberMe"`
	}

	// Validate request
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request",
			"errors": gin.H{
				"email": "Email is required and must be valid",
				"code":  "OTP code is required and must be 6 digits",
			},
		})
		return
	}

	// Normalize inputs
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)

	// Validate email format
	if !isValidEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid email format",
			"errors": gin.H{
				"email": "Please provide a valid email address (e.g., user@example.com)",
			},
		})
		return
	}

	// Validate OTP code format
	if len(req.Code) != 6 || !regexp.MustCompile(`^\d{6}$`).MatchString(req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid OTP code",
			"errors": gin.H{
				"code": "OTP code must be exactly 6 digits. Check your email.",
			},
		})
		return
	}

	// Verify OTP
	user, authMethod, err := h.service.VerifyLoginMFA(
		req.Email,
		req.Code,
		"",
		"",
		h.loginMetadata(c),
	)

	if err != nil {
		errMsg := strings.ToLower(err.Error())
		
		// User not found
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "account") {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Verification failed",
				"errors": gin.H{
					"email": "No account found with this email address",
				},
			})
			return
		}

		// Invalid/expired code
		if strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "expired") {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Incorrect or expired OTP",
				"errors": gin.H{
					"code": "The OTP code is incorrect or has expired. Please try again or request a new code.",
				},
			})
			return
		}

		// Generic error
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Verification failed",
			"errors": gin.H{
				"general": "An error occurred. Please try again.",
			},
		})
		return
	}

	// Success
	if err := h.issueAuthenticatedSession(c, user, req.RememberMe, authMethod); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Session creation failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login successful",
		"data": gin.H{
			"user": authUserPayload(user),
		},
	})
}
```

### Part 3: Frontend - Implement Retry Logic

**File**: `src/lib/api/auth.ts` (update apiClient)

```typescript
// Add retry logic with exponential backoff
async function fetchWithRetry(
  url: string,
  options: RequestInit,
  maxRetries = 3
): Promise<Response> {
  let lastError: Error;

  for (let attempt = 0; attempt < maxRetries; attempt++) {
    try {
      const response = await fetch(url, {
        ...options,
        signal: AbortSignal.timeout(30000), // 30 sec timeout
      });

      // Don't retry on 4xx client errors
      if (response.status >= 400 && response.status < 500) {
        return response;
      }

      // Retry on 5xx server errors and 429 rate limit
      if ((response.status >= 500 || response.status === 429) && attempt < maxRetries - 1) {
        const retryAfter = response.headers.get('Retry-After');
        const delay = retryAfter ? parseInt(retryAfter) * 1000 : Math.pow(2, attempt) * 1000;
        
        await new Promise(resolve => setTimeout(resolve, delay));
        continue;
      }

      return response;
    } catch (error) {
      lastError = error as Error;
      
      // Don't retry on network errors
      if (attempt < maxRetries - 1) {
        const delay = Math.pow(2, attempt) * 1000;
        await new Promise(resolve => setTimeout(resolve, delay));
        continue;
      }
    }
  }

  throw lastError;
}

// Update auth refresh to use retry logic
async function refreshAuth() {
  try {
    const response = await fetchWithRetry(
      'https://api.wisdomchurchhq.org/api/v1/auth/me',
      {
        method: 'GET',
        credentials: 'include',
        headers: {
          'Accept': 'application/json',
        },
      }
    );

    if (!response.ok) {
      // Log out on 401/403
      if (response.status === 401 || response.status === 403) {
        logout();
        return null;
      }
      
      // Show error on 429
      if (response.status === 429) {
        showError('Too many requests. Please wait a moment.');
        return null;
      }

      return null;
    }

    return response.json();
  } catch (error) {
    console.error('Auth refresh failed:', error);
    return null;
  }
}

// Update OTP verification with validation
async function verifyOTP(email: string, code: string) {
  // Validate before sending
  if (!email || !email.includes('@')) {
    throw new Error('Invalid email format');
  }
  
  if (!code || !/^\d{6}$/.test(code)) {
    throw new Error('OTP must be exactly 6 digits');
  }

  const response = await fetchWithRetry(
    'https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp',
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      credentials: 'include',
      body: JSON.stringify({
        email: email.toLowerCase().trim(),
        code: code.trim(),
      }),
    }
  );

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.message || 'Verification failed');
  }

  return response.json();
}
```

### Part 4: Frontend - Better Error Handling

**File**: `src/hooks/useAuth.ts` (or similar)

```typescript
export function useAuth() {
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  async function handleOTPSubmit(email: string, code: string) {
    try {
      // Clear previous errors
      setError(null);
      setFieldErrors({});

      // Validate
      const errors: Record<string, string> = {};
      if (!email) errors.email = 'Email is required';
      if (!code) errors.code = 'OTP code is required';
      if (code && !/^\d{6}$/.test(code)) errors.code = 'OTP must be 6 digits';

      if (Object.keys(errors).length > 0) {
        setFieldErrors(errors);
        return;
      }

      // Submit
      const result = await verifyOTP(email, code);
      
      // Success
      localStorage.setItem('auth_token', result.data.token);
      window.location.href = '/dashboard';

    } catch (error) {
      const message = error instanceof Error ? error.message : 'Verification failed';
      
      // Show specific field errors or general error
      if (message.includes('email')) {
        setFieldErrors({ email: message });
      } else if (message.includes('code') || message.includes('OTP')) {
        setFieldErrors({ code: message });
      } else if (message.includes('Too many')) {
        setError('Too many requests. Please wait a moment and try again.');
      } else {
        setError(message);
      }
    }
  }

  return { handleOTPSubmit, error, fieldErrors };
}
```

---

## Implementation Steps

### Step 1: Backend (Priority: CRITICAL)
```bash
# 1. Create rate limiting middleware
# 2. Update VerifyLoginOTP with better validation
# 3. Test with curl:
curl -X POST https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","code":"123456"}'

# Expected: Clear error message
```

### Step 2: Frontend (Priority: HIGH)
```bash
# 1. Update API client with retry logic
# 2. Add input validation before sending
# 3. Improve error display to users
# 4. Test in staging environment
```

### Step 3: Testing (Priority: HIGH)
```bash
# Test rate limiting
for i in {1..20}; do
  curl -X GET https://api.wisdomchurchhq.org/api/v1/auth/me
  sleep 0.1
done

# Test OTP with invalid input
curl -X POST https://api.wisdomchurchhq.org/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  -d '{"email":"invalid","code":"123"}'
```

---

## Expected Results After Fix

### Before Fix (Current Production)
```
❌ GET /auth/me → 429 Too Many Requests (no helpful message)
❌ POST /verify-otp → 400 Bad Request (no error details)
❌ Frontend: Generic errors, users confused
❌ Users locked out, forced logouts
```

### After Fix
```
✅ GET /auth/me → 200 OK (with retry logic)
✅ POST /verify-otp → 400 with helpful error messages
✅ Frontend: Clear field errors, retry on server errors
✅ Users can successfully authenticate
✅ Better error messages help users understand issues
```

---

## Rollout Plan

### Phase 1: Backend (30 min)
1. Update rate limiting configuration
2. Improve error messages in VerifyLoginOTP
3. Deploy to staging
4. Test thoroughly

### Phase 2: Frontend (20 min)
1. Add retry logic to API client
2. Improve error handling
3. Deploy to staging
4. User acceptance testing

### Phase 3: Production (5 min)
1. Deploy backend to production
2. Monitor for 429 errors
3. Deploy frontend to production
4. Monitor user authentication flow

### Phase 4: Monitoring
1. Alert on 429 rate limit errors
2. Alert on 400 bad request errors
3. Track auth success rate
4. Monitor user login completion rate

---

## Success Metrics

After fix, track:
- ✅ Auth success rate > 99%
- ✅ Zero 429 errors on /auth/me
- ✅ User login completion rate > 95%
- ✅ Average OTP verification time < 500ms
- ✅ Zero user complaints about auth

---

## References

- Rate Limiting Best Practices: https://tools.ietf.org/html/draft-polli-ratelimit-headers
- Retry Strategy: Exponential backoff with jitter
- HTTP Status Codes: https://httpwg.org/specs/rfc7231.html
