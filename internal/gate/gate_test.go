package gate_test

import (
	"testing"

	"github.com/sam33339999/wikibuild/internal/gate"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/stretchr/testify/require"
)

func TestDecide_PublicAlwaysAllowed(t *testing.T) {
	for _, isAdmin := range []bool{false, true} {
		for _, unlocked := range []bool{false, true} {
			d := gate.Decide(gate.AccessInput{
				Status: model.StatusPublished, Visibility: model.VisibilityPublic,
				IsAdmin: isAdmin, Unlocked: unlocked,
			})
			require.Equal(t, gate.Allow, d, "public must always allow (admin=%v unlocked=%v)", isAdmin, unlocked)
		}
	}
}

func TestDecide_Private_NotAdmin_Is404(t *testing.T) {
	d := gate.Decide(gate.AccessInput{
		Status: model.StatusPublished, Visibility: model.VisibilityPrivate,
		IsAdmin: false, Unlocked: true,
	})
	require.Equal(t, gate.NotFound, d, "private hides existence from non-admins even if unlocked")
}

func TestDecide_Private_AdminAllowed(t *testing.T) {
	d := gate.Decide(gate.AccessInput{
		Status: model.StatusPublished, Visibility: model.VisibilityPrivate, IsAdmin: true,
	})
	require.Equal(t, gate.Allow, d)
}

func TestDecide_Protected_NotAdmin_NotUnlocked_AsksPassword(t *testing.T) {
	d := gate.Decide(gate.AccessInput{
		Status: model.StatusPublished, Visibility: model.VisibilityProtected,
		IsAdmin: false, Unlocked: false,
	})
	require.Equal(t, gate.Password, d)
}

func TestDecide_Protected_NotAdmin_Unlocked_Allowed(t *testing.T) {
	d := gate.Decide(gate.AccessInput{
		Status: model.StatusPublished, Visibility: model.VisibilityProtected,
		IsAdmin: false, Unlocked: true,
	})
	require.Equal(t, gate.Allow, d)
}

func TestDecide_Protected_AdminBypassesPassword(t *testing.T) {
	d := gate.Decide(gate.AccessInput{Visibility: model.VisibilityProtected, IsAdmin: true, Unlocked: false})
	require.Equal(t, gate.Allow, d, "admin sees protected without unlocking")
}

func TestDecide_DraftIsNotFound(t *testing.T) {
	// Drafts are never public regardless of visibility; the gate treats a
	// non-published article as not found for non-admins.
	for _, vis := range []model.Visibility{model.VisibilityPublic, model.VisibilityProtected, model.VisibilityPrivate} {
		d := gate.Decide(gate.AccessInput{
			Status: model.StatusDraft, Visibility: vis, IsAdmin: false,
		})
		require.Equal(t, gate.NotFound, d, "draft %s not shown to non-admin", vis)
	}
}

func TestDecide_Draft_AdminAllowed(t *testing.T) {
	d := gate.Decide(gate.AccessInput{Status: model.StatusDraft, Visibility: model.VisibilityPublic, IsAdmin: true})
	require.Equal(t, gate.Allow, d, "admin can preview drafts")
}
