package auth_test

import (
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/stretchr/testify/require"
)

func newTestLimiter(t0 time.Time) (*auth.LoginLimiter, *clock.Fake) {
	fc := clock.NewFake(t0)
	l := auth.NewLoginLimiter(fc, auth.LimiterConfig{
		MaxAttempts: 3,
		Window:      10 * time.Minute,
		Lockout:     15 * time.Minute,
	})
	return l, fc
}

func TestLoginLimiter_NotLockedInitially(t *testing.T) {
	l, _ := newTestLimiter(time.Unix(1_700_000_000, 0))
	require.False(t, l.IsLocked("1.2.3.4"))
}

func TestLoginLimiter_LocksAfterMaxAttempts(t *testing.T) {
	l, _ := newTestLimiter(time.Unix(1_700_000_000, 0))
	l.RegisterFailure("1.2.3.4")
	l.RegisterFailure("1.2.3.4")
	require.False(t, l.IsLocked("1.2.3.4"), "2 of 3 attempts must not lock")

	l.RegisterFailure("1.2.3.4")
	require.True(t, l.IsLocked("1.2.3.4"), "3rd attempt must lock")
}

func TestLoginLimiter_LockoutReleasesAfterDuration(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	l, fc := newTestLimiter(t0)

	for i := 0; i < 3; i++ {
		l.RegisterFailure("1.2.3.4")
	}
	require.True(t, l.IsLocked("1.2.3.4"))

	fc.Set(t0.Add(16 * time.Minute))
	require.False(t, l.IsLocked("1.2.3.4"), "must unlock after lockout elapses")
}

func TestLoginLimiter_LockedAttemptExtendsLockout(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	l, fc := newTestLimiter(t0)

	for i := 0; i < 3; i++ {
		l.RegisterFailure("1.2.3.4")
	}
	// Halfway through the lockout, another attempt should push it out.
	fc.Set(t0.Add(7 * time.Minute))
	require.True(t, l.IsLocked("1.2.3.4"))
	l.RegisterFailure("1.2.3.4")

	// At original expiry, still locked because it was extended at +7min.
	fc.Set(t0.Add(15 * time.Minute))
	require.True(t, l.IsLocked("1.2.3.4"), "extended attempt keeps it locked past original expiry")

	fc.Set(t0.Add(22 * time.Minute))
	require.False(t, l.IsLocked("1.2.3.4"))
}

func TestLoginLimiter_FailuresOutsideWindowDontCount(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	l, fc := newTestLimiter(t0)

	l.RegisterFailure("1.2.3.4")
	l.RegisterFailure("1.2.3.4")
	// Advance past the window: those two attempts should age out.
	fc.Set(t0.Add(11 * time.Minute))
	l.RegisterFailure("1.2.3.4")
	require.False(t, l.IsLocked("1.2.3.4"), "aged-out attempts must not contribute to lock")
}

func TestLoginLimiter_SuccessClearsAttempts(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	l, _ := newTestLimiter(t0)

	l.RegisterFailure("1.2.3.4")
	l.RegisterFailure("1.2.3.4")
	l.RegisterSuccess("1.2.3.4")
	l.RegisterFailure("1.2.3.4")
	require.False(t, l.IsLocked("1.2.3.4"), "success resets the attempt counter")
}

func TestLoginLimiter_KeysAreIndependent(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	l, _ := newTestLimiter(t0)

	for i := 0; i < 3; i++ {
		l.RegisterFailure("1.2.3.4")
	}
	require.True(t, l.IsLocked("1.2.3.4"))
	require.False(t, l.IsLocked("5.6.7.8"), "other keys unaffected")
}
