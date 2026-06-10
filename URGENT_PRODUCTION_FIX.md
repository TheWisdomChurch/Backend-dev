# 🚨 URGENT: Production Fix - Auth Failing

**Status**: 🔴 CRITICAL - Users cannot authenticate
**Date**: 2026-06-10
**Severity**: P0 - Production Down

---

## What's Broken Right Now

### 1. Rate Limiting (429 Too Many Requests)
```
Error: GET /api/v1/auth/me 429 Too Many Requests
Impact: Users logged out repeatedly
Cause: Backend rate limiting auth refresh too aggressively
```

### 2. OTP Verification (400 Bad Request)
```
Error: POST /api/v1/auth/login/verify-otp 400 Bad Request
Impact: Users cannot complete login
Cause: No validation, no helpful error messages
```

---

## Immediate Actions (Next 30 Minutes)

### ✅ Step 1: Deploy Rate Limit Fix (Backend - 5 min)

**What to do:**
1. Add `internal/middleware/rate_limit.go` (already created)
2. Update `cmd/api/main.go` to register rate limiting on auth routes

**File**: `cmd/api/main.go` - Add this in route setup:

```go
// Apply rate limiting to sensitive endpoints
authAPI := api.Group("/auth")
authAPI.Use(middleware.RateLimitMiddleware("/api/v1/auth/me"))
authAPI.GET("/me", authHandler.GetCurrentUser)

authAPI.Use(middleware.RateLimitMiddleware("/api/v1/auth/login/verify-otp"))
authAPI.POST("/login/verify-otp", authHandler.VerifyLoginOTP)
```

**Verify:**
```bash
go build ./cmd/api
# Should compile successfully
```

### ✅ Step 2: Fix OTP Validation (Backend - 10 min)

**File**: `internal/handlers/auth.go` line ~1180

Replace the `VerifyLoginOTP` function with this:

```go
func (h *AuthHandler) VerifyLoginOTP(c *gin.Context) {
	var req struct {
		Email      string `json:"email"`
		Code       string `json:"code"`
		RememberMe bool   `json:"rememberMe"`
	}

	// Parse request
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request format",
			"errors": gin.H{
				"email": "Email is required",
				"code":  "OTP code is required (6 digits)",
			},
		})
		return
	}

	// Normalize
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)

	// Validate email
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid email",
			"errors": gin.H{
				"email": "Please provide a valid email address",
			},
		})
		return
	}

	// Validate OTP code
	if len(req.Code) != 6 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid OTP code",
			"errors": gin.H{
				"code": "OTP must be exactly 6 digits (check your email)",
			},
		})
		return
	}

	// Check if all digits
	for _, ch := range req.Code {
		if ch < '0' || ch > '9' {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Invalid OTP code",
				"errors": gin.H{
					"code": "OTP must contain only numbers",
				},
			})
			return
		}
	}

	// Verify with service
	user, authMethod, err := h.service.VerifyLoginMFA(
		req.Email,
		req.Code,
		"",
		"",
		h.loginMetadata(c),
	)

	if err != nil {
		errMsg := strings.ToLower(err.Error())

		// Check error type and return appropriate message
		if strings.Contains(errMsg, "not found") {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Verification failed",
				"errors": gin.H{
					"email": "No account found with this email",
				},
			})
			return
		}

		if strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "incorrect") {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Incorrect OTP code",
				"errors": gin.H{
					"code": "The OTP code is incorrect. Check your email and try again.",
				},
			})
			return
		}

		// Generic error
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Verification failed",
			"errors": gin.H{
				"general": "An error occurred. Please try again.",
			},
		})
		return
	}

	if user == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid response",
		})
		return
	}

	// Create session
	if err := h.issueAuthenticatedSession(c, user, req.RememberMe, authMethod); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Session creation failed",
		})
		return
	}

	// Success
	accessStatus, accessCode, nextStep := deriveAccessStatus(user, authMethod)
	responseData := authUserPayload(user)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login successful",
		"data": gin.H{
			"user": responseData,
		},
		"meta": gin.H{
			"authenticated": true,
			"access_status": accessStatus,
			"access_code":   accessCode,
			"next_step":     nextStep,
		},
	})
}
```

**Test:**
```bash
curl -X POST http://localhost:8080/api/v1/auth/login/verify-otp \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","code":"12345"}'
  
# Should return 400 with error message about code length
```

### ✅ Step 3: Deploy (5 min)

```bash
# Build
go build -o bin/wisdomhousebe ./cmd/api

# Test
./bin/wisdomhousebe &

# Verify endpoints work
curl http://localhost:8080/api/v1/health

# Kill and redeploy to production
# docker build -t wisdom-house-backend:latest -f dockerfile .
# docker push to registry
```

---

## Frontend Changes (Next 15 Minutes)

### Update Auth Retry Logic

**File**: `Frontend-dev/src/lib/api.ts`

Add at the top of the file:

```typescript
// Retry logic for rate limiting
async function fetchWithRetry(
  url: string,
  options: RequestInit,
  maxRetries = 3
): Promise<Response> {
  for (let attempt = 0; attempt < maxRetries; attempt++) {
    try {
      const response = await fetch(url, {
        ...options,
        signal: AbortSignal.timeout(30000),
      });

      // Don't retry 4xx errors except 429
      if (response.status >= 400 && response.status < 500 && response.status !== 429) {
        return response;
      }

      // Retry on 5xx and 429
      if ((response.status >= 500 || response.status === 429) && attempt < maxRetries - 1) {
        const delay = Math.pow(2, attempt) * 1000;
        await new Promise(r => setTimeout(r, delay));
        continue;
      }

      return response;
    } catch (error) {
      if (attempt < maxRetries - 1) {
        const delay = Math.pow(2, attempt) * 1000;
        await new Promise(r => setTimeout(r, delay));
      } else {
        throw error;
      }
    }
  }
  throw new Error('Max retries exceeded');
}
```

Update the request function to use `fetchWithRetry` instead of direct `fetch`.

---

## Testing Checklist

### Backend Tests
- [ ] OTP with invalid email → 400 with error message
- [ ] OTP with invalid code (5 digits) → 400 with error message
- [ ] OTP with valid email/code → 200 with success
- [ ] Multiple rapid requests to /auth/me → First 10 succeed, rest get 429
- [ ] Rate limit error includes Retry-After header

### Frontend Tests
- [ ] Submit OTP form with missing email → Shows error
- [ ] Submit OTP form with invalid code → Shows error
- [ ] Submit OTP form with valid data → Logs in successfully
- [ ] Rapid auth refreshes → First succeed, then wait

### Production Tests
- [ ] Real user can log in via OTP
- [ ] Multiple users can log in concurrently
- [ ] No 429 errors in console
- [ ] No 400 errors on valid input

---

## Rollout Steps

### 1. Deploy Backend (Production)
```bash
# 1. Build and test locally
go build ./cmd/api

# 2. Build Docker image
docker build -t wisdom-house-backend:prod -f dockerfile .

# 3. Push to registry
docker tag wisdom-house-backend:prod ghcr.io/thewisdomchurch/wisdom-api:prod
docker push ghcr.io/thewisdomchurch/wisdom-api:prod

# 4. Deploy (via your CD pipeline or manually)
# Ensure new image is running
```

### 2. Monitor Backend
```bash
# Watch logs for errors
docker logs wisdom-house-backend -f

# Monitor metrics
# - Check for 429 errors (should be minimal)
# - Check for 400 errors (should have helpful messages)
# - Check auth success rate
```

### 3. Deploy Frontend
```bash
# 1. Update src/lib/api.ts with retry logic
# 2. Build
npm run build

# 3. Deploy
# Push to production via your pipeline
```

### 4. Verify in Production
```bash
# Check admin portal can log in
# Check no errors in browser console
# Check user feedback
```

---

## Monitoring Alert Setup

### Alert on 429 Errors
```
Condition: Count of 429 responses > 10 per minute
Action: Notify team
Threshold: WARNING > 5, CRITICAL > 10
```

### Alert on 400 Errors
```
Condition: Count of 400 on /verify-otp > 50 per minute
Action: Investigate
Note: Some 400s are normal (wrong code), look for spike
```

### Alert on Auth Failures
```
Condition: Auth success rate < 95%
Action: Page oncall
Threshold: 95% is acceptable, < 90% is critical
```

---

## Success Metrics

After fix:
- ✅ 0 user complaints about auth
- ✅ Auth success rate > 99%
- ✅ < 1% rate limit errors
- ✅ Clear error messages in browser console
- ✅ Users can complete login in < 5 seconds

---

## Rollback Plan

If issues occur:

```bash
# 1. Revert backend to previous image
docker rollback wisdom-house-backend

# 2. Clear frontend cache
# CDN cache clear or manual cache purge

# 3. Test
curl https://api.wisdomchurchhq.org/api/v1/health

# 4. Monitor for 30 minutes
# Check error rates return to normal
```

---

## Timeline

| Task | Time | Owner |
|------|------|-------|
| Code changes (backend) | 10 min | Backend team |
| Build & test | 5 min | Backend team |
| Deploy to staging | 5 min | DevOps |
| Smoke test | 5 min | QA |
| Deploy to production | 5 min | DevOps |
| Monitor | 30 min | Oncall |
| Frontend updates | 15 min | Frontend team |
| Frontend deploy | 5 min | DevOps |
| **Total** | **~50 min** | |

---

## Questions?

If anything is unclear, ask before deploying!

This fix will:
- ✅ Eliminate 429 rate limit errors
- ✅ Fix 400 OTP verification errors
- ✅ Provide helpful error messages to users
- ✅ Improve overall auth reliability
