# Debug GitHub Provider Issue

## Check Your JWT Token

Open your browser console and run this:

```javascript
// Get current session
const { data: { session } } = await supabase.auth.getSession();

// Decode JWT token
const token = session.access_token;
const parts = token.split('.');
const payload = JSON.parse(atob(parts[1]));

console.log('JWT Payload:', payload);
console.log('Provider:', payload.app_metadata?.provider);
console.log('User metadata:', payload.user_metadata);
```

## What to Look For

Your JWT should have:
```json
{
  "sub": "user-id-here",
  "email": "your@email.com",
  "app_metadata": {
    "provider": "github",  // ← Should be "github" for GitHub auth
    "providers": ["github"]
  },
  "user_metadata": {
    "avatar_url": "https://avatars.githubusercontent.com/...",
    "full_name": "Your Name"
  }
}
```

## If Provider is "email" in JWT

This means Supabase thinks you're an email user. Possible reasons:

### Scenario 1: Email Account Created First
You created an account with email/password first, THEN tried GitHub OAuth with the same email.

**Solution:**
1. Delete the email account from Supabase Dashboard
2. Sign up fresh with GitHub OAuth only

### Scenario 2: Provider Field Missing
The JWT doesn't have the provider in `app_metadata.provider`.

**Solution:** Update backend to check multiple locations for provider.

### Scenario 3: Account Linking
Supabase linked GitHub to existing email account but kept email as primary provider.

**Solution:** Manually update the provider in Supabase Dashboard.

---

## Fix 1: Delete Email Account and Use GitHub

**In Supabase Dashboard:**
1. Go to **Authentication** → **Users**
2. Find your user (keshavdv241@gmail.com)
3. Click the three dots → **Delete User**
4. In your frontend, sign in with GitHub OAuth
5. Check the database - should now show `provider: "github"`

---

## Fix 2: Manually Update Provider in Database

**If you want to keep the account:**

```sql
-- Check current provider
SELECT user_id, email, provider FROM auth.users WHERE email = 'keshavdv241@gmail.com';

-- Update to GitHub
UPDATE auth.users
SET raw_app_meta_data = jsonb_set(raw_app_meta_data, '{provider}', '"github"')
WHERE email = 'keshavdv241@gmail.com';

-- Also update in your custom users table
UPDATE users
SET provider = 'github'
WHERE email = 'keshavdv241@gmail.com';
```

---

## Fix 3: Update Backend to Handle Multiple Provider Locations

The JWT token might have provider in a different location. Let's update the middleware:

**File:** `control-plane/internal/auth/middleware.go`

Update the provider extraction logic:

```go
// Try multiple locations for provider
provider := strings.ToLower(strings.TrimSpace(claims.Provider))
if provider == "" {
    provider = strings.ToLower(strings.TrimSpace(claims.AppMeta.Provider))
}

// Also check user_metadata if still not found
if provider == "" {
    if userMeta, ok := claims.UserMeta["provider"].(string); ok {
        provider = strings.ToLower(strings.TrimSpace(userMeta))
    }
}

// Check if providers array exists
if provider == "" && claims.AppMeta.Providers != nil && len(claims.AppMeta.Providers) > 0 {
    provider = strings.ToLower(strings.TrimSpace(claims.AppMeta.Providers[0]))
}

// Default to email if still not found
if provider == "" {
    provider = "email"
}

logs.Debugf("auth", "detected provider: %s from token", provider)
```

---

## Fix 4: Check Supabase User Identities

In Supabase Dashboard:
1. Go to **Authentication** → **Users**
2. Click on your user (keshavdv241@gmail.com)
3. Scroll to **Identities** section
4. You should see identities listed (email, github, etc.)

**If you see both "email" and "github" identities:**
- The user was created with email first
- GitHub was linked later
- The primary provider remains "email"

**To fix:**
- Delete the "email" identity
- Keep only "github" identity
- Or delete user and recreate with GitHub only

---

## Test Your GitHub OAuth Flow

Create a test script to verify GitHub OAuth:

```html
<!DOCTYPE html>
<html>
<head>
  <title>Test GitHub OAuth</title>
  <script src="https://cdn.jsdelivr.net/npm/@supabase/supabase-js@2"></script>
</head>
<body>
  <h1>GitHub OAuth Test</h1>
  <button onclick="signInWithGitHub()">Sign in with GitHub</button>
  <button onclick="checkSession()">Check Session</button>
  <pre id="output"></pre>

  <script>
    const supabase = supabase.createClient(
      'YOUR_SUPABASE_URL',
      'YOUR_SUPABASE_ANON_KEY'
    );

    async function signInWithGitHub() {
      const { data, error } = await supabase.auth.signInWithOAuth({
        provider: 'github',
        options: {
          scopes: 'repo user',
          redirectTo: window.location.origin
        }
      });

      if (error) {
        document.getElementById('output').textContent = 'Error: ' + error.message;
      }
    }

    async function checkSession() {
      const { data: { session } } = await supabase.auth.getSession();

      if (!session) {
        document.getElementById('output').textContent = 'Not signed in';
        return;
      }

      // Decode JWT
      const token = session.access_token;
      const parts = token.split('.');
      const payload = JSON.parse(atob(parts[1]));

      const output = {
        email: session.user.email,
        provider_from_user: session.user.app_metadata?.provider,
        provider_from_jwt: payload.app_metadata?.provider,
        identities: session.user.identities,
        full_jwt: payload
      };

      document.getElementById('output').textContent = JSON.stringify(output, null, 2);
    }
  </script>
</body>
</html>
```

---

## Expected vs Actual

### ✅ Expected (GitHub OAuth)
```
provider: "github"
app_metadata.provider: "github"
identities: [{ provider: "github", ... }]
```

### ❌ Actual (What you're seeing)
```
provider: "email"
app_metadata.provider: "email"
identities: [{ provider: "email", ... }]
```

---

## Recommended Solution

**The cleanest fix:**

1. **Delete both test accounts** from Supabase Dashboard:
   - keshavkumar.232803108@vcet.edu.in
   - keshavdv241@gmail.com

2. **Clear your browser cookies/storage** for your frontend app

3. **Sign up fresh with GitHub OAuth ONLY** - don't use email signup

4. **Verify in database:**
   ```sql
   SELECT user_id, email, provider FROM users;
   ```
   Should show `provider: "github"`

5. **Test backend API:**
   ```bash
   curl http://localhost:8080/auth/whoami \
     -H "Authorization: Bearer YOUR_TOKEN"
   ```
   Should return `provider: "github"`

This ensures a clean GitHub-only authentication flow!
