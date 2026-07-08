package authutil

import "testing"

func TestPasswordPolicy_Validate_DefaultPolicy(t *testing.T) {
	p := DefaultPasswordPolicy()

	cases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid strong password", "Str0ng!Passw0rd", false},
		{"too short", "Sh0rt!", true},
		{"missing uppercase", "no-upper-1!aaaa", true},
		{"missing lowercase", "NO-LOWER-1!AAAA", true},
		{"missing digit", "NoDigitsHere!!", true},
		{"missing special character", "NoSpecialChars123", true},
		{"exactly minimum length and all classes", "Aa1!Aa1!Aa1!", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := p.Validate(tc.password)
			if tc.wantErr && err == nil {
				t.Errorf("Validate(%q) = nil, want an error", tc.password)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate(%q) = %v, want nil", tc.password, err)
			}
		})
	}
}

func TestPasswordPolicy_Validate_NilPolicyAlwaysPasses(t *testing.T) {
	var p *PasswordPolicy
	if err := p.Validate(""); err != nil {
		t.Errorf("nil policy should never reject a password, got: %v", err)
	}
}

func TestPolicyFromConfig_AppliesMinLengthWithFallback(t *testing.T) {
	p := PolicyFromConfig(20)
	if p.MinLength != 20 {
		t.Errorf("MinLength = %d, want 20", p.MinLength)
	}

	fallback := PolicyFromConfig(0)
	if fallback.MinLength != 12 {
		t.Errorf("MinLength with non-positive input = %d, want fallback of 12", fallback.MinLength)
	}

	negative := PolicyFromConfig(-5)
	if negative.MinLength != 12 {
		t.Errorf("MinLength with negative input = %d, want fallback of 12", negative.MinLength)
	}
}

func TestPasswordPolicy_Validate_RespectsDisabledRequirements(t *testing.T) {
	p := &PasswordPolicy{MinLength: 4, RequireUpper: false, RequireLower: false, RequireDigit: false, RequireSpecial: false}

	if err := p.Validate("abcd"); err != nil {
		t.Errorf("expected a policy with all character-class requirements disabled to accept a plain password, got: %v", err)
	}
	if err := p.Validate("abc"); err == nil {
		t.Error("expected a password shorter than MinLength to still be rejected")
	}
}
