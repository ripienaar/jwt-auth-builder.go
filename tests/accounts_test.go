package tests

import (
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
	authb "github.com/synadia-io/jwt-auth-builder.go"
)

func (suite *ProviderSuite) Test_AccountsCrud() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	accounts := o.Accounts().List()
	require.Nil(t, err)
	require.Equal(t, 0, len(accounts))

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)
	b, err := o.Accounts().Add("B")
	require.NoError(t, err)

	x := o.Accounts().Get("X")
	require.Nil(t, err)
	require.Nil(t, x)

	x = o.Accounts().Get("A")
	require.Nil(t, err)
	require.NotNil(t, x)
	require.Equal(t, "A", x.Name())

	accounts = o.Accounts().List()
	require.Nil(t, err)
	require.Equal(t, 2, len(accounts))
	require.Contains(t, accounts, a)
	require.Contains(t, accounts, b)

	require.NoError(t, o.Accounts().Delete("A"))
	accounts = o.Accounts().List()
	require.Nil(t, err)
	require.Equal(t, 1, len(accounts))
	require.Contains(t, accounts, b)

	require.NoError(t, auth.Commit())
	require.True(t, suite.Store.AccountExists("O", "B"))
	require.False(t, suite.Store.AccountExists("O", "A"))
}

func (suite *ProviderSuite) Test_AccountsBasics() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)

	ai := a.(*authb.AccountData)
	require.Equal(t, ai.Claim.Subject, a.Subject())
	require.Equal(t, o.Subject(), a.Issuer())

	acct, err := authb.NewAccountFromJWT(a.JWT())
	require.NoError(t, err)
	require.Equal(t, acct.Subject(), a.Subject())
	err = acct.SetExpiry(time.Now().Unix())
	require.Equal(t, err, fmt.Errorf("account is read-only"))
}

func (suite *ProviderSuite) Test_AccountLimitsSetter() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)

	require.NoError(t, a.Limits().SetMaxExports(10))
	require.Equal(t, a.Limits().MaxExports(), int64(10))
	require.Equal(t, a.Limits().MaxImports(), int64(-1))

	type operatorLimitsManager interface {
		OperatorLimits() jwt.OperatorLimits
		SetOperatorLimits(limits jwt.OperatorLimits) error
	}

	al := a.Limits().(operatorLimitsManager).OperatorLimits()
	require.Equal(t, al.Exports, int64(10))

	al.Exports = 20
	al.Imports = 10
	require.NoError(t, a.Limits().(operatorLimitsManager).SetOperatorLimits(al))
	require.Equal(t, a.Limits().MaxExports(), int64(20))
	require.Equal(t, a.Limits().MaxImports(), int64(10))
}

func (suite *ProviderSuite) Test_UserPermissionLimitsSetter() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)

	user, err := a.Users().Add("BOB", "")
	require.NoError(t, err)

	require.Equal(t, user.MaxSubscriptions(), int64(-1))
	require.Empty(t, user.PubPermissions().Allow())

	type userLimitsManager interface {
		UserPermissionLimits() jwt.UserPermissionLimits
		SetUserPermissionLimits(limits jwt.UserPermissionLimits) error
	}

	limits := jwt.UserPermissionLimits{}
	limits.Permissions.Pub.Allow = []string{"test.>"}
	limits.NatsLimits.Subs = 1000

	err = user.(userLimitsManager).SetUserPermissionLimits(limits)
	require.NoError(t, err)

	require.Equal(t, user.MaxSubscriptions(), int64(1000))
	require.Equal(t, user.PubPermissions().Allow(), []string{"test.>"})
}

func (suite *ProviderSuite) Test_ScopedUserPermissionLimitsSetter() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)

	scope, err := a.ScopedSigningKeys().AddScope("test")
	require.NoError(t, err)

	user, err := a.Users().Add("BOB", scope.Key())
	require.NoError(t, err)

	require.Equal(t, scope.MaxSubscriptions(), int64(-1))
	require.Empty(t, scope.PubPermissions().Allow())

	limits := jwt.UserPermissionLimits{}
	limits.Permissions.Pub.Allow = []string{"test.>"}
	limits.NatsLimits.Subs = 1000

	type userLimitsManager interface {
		UserPermissionLimits() jwt.UserPermissionLimits
		SetUserPermissionLimits(limits jwt.UserPermissionLimits) error
	}

	err = user.(userLimitsManager).SetUserPermissionLimits(limits)
	require.Errorf(t, err, "user is scoped")

	err = scope.(userLimitsManager).SetUserPermissionLimits(limits)
	require.NoError(t, err)

	require.Equal(t, scope.MaxSubscriptions(), int64(1000))
	require.Equal(t, scope.PubPermissions().Allow(), []string{"test.>"})
}

func setupTestWithOperatorAndAccount(p *ProviderSuite) (authb.Auth, authb.Operator, authb.Account) {
	t := p.T()
	auth, err := authb.NewAuth(p.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)
	require.NoError(t, auth.Commit())

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)
	return auth, o, a
}

func (suite *ProviderSuite) Test_ScopedPermissionsMaxSubs() {
	t := suite.T()
	auth, _, a := setupTestWithOperatorAndAccount(suite)
	s, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	require.NoError(t, s.SetMaxSubscriptions(10))
	require.Equal(t, int64(10), s.MaxSubscriptions())
	require.NoError(t, auth.Commit())

	require.NoError(t, auth.Reload())

	o := auth.Operators().Get("O")
	require.NoError(t, err)
	require.NotNil(t, o)

	a = o.Accounts().Get("A")
	require.NoError(t, err)
	require.NotNil(t, a)

	s = a.ScopedSigningKeys().GetScopeByRole("admin")
	require.NotNil(t, s)
	require.Equal(t, int64(10), s.MaxSubscriptions())
}

func (suite *ProviderSuite) Test_ScopedPermissionsMaxPayload() {
	t := suite.T()
	auth, _, a := setupTestWithOperatorAndAccount(suite)
	s, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	require.NoError(t, s.SetMaxPayload(101))
	require.Equal(t, int64(101), s.MaxPayload())
	require.NoError(t, auth.Commit())

	require.NoError(t, auth.Reload())

	o := auth.Operators().Get("O")
	require.NoError(t, err)
	require.NotNil(t, o)

	a = o.Accounts().Get("A")
	require.NoError(t, err)
	require.NotNil(t, a)

	s = a.ScopedSigningKeys().GetScopeByRole("admin")
	require.NotNil(t, s)
	require.Equal(t, int64(101), s.MaxPayload())
}

func (suite *ProviderSuite) Test_ScopedPermissionsMaxData() {
	t := suite.T()
	auth, _, a := setupTestWithOperatorAndAccount(suite)
	s, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	require.NoError(t, s.SetMaxData(4123))
	require.Equal(t, int64(4123), s.MaxData())
	require.NoError(t, auth.Commit())

	require.NoError(t, auth.Reload())

	o := auth.Operators().Get("O")
	require.NoError(t, err)
	require.NotNil(t, o)

	a = o.Accounts().Get("A")
	require.NoError(t, err)
	require.NotNil(t, a)

	s = a.ScopedSigningKeys().GetScopeByRole("admin")
	require.NotNil(t, s)
	require.Equal(t, int64(4123), s.MaxData())
}

func (suite *ProviderSuite) Test_ScopedPermissionsBearerToken() {
	t := suite.T()
	auth, _, a := setupTestWithOperatorAndAccount(suite)
	s, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	require.NoError(t, s.SetBearerToken(true))
	require.True(t, s.BearerToken())
	require.NoError(t, auth.Commit())

	require.NoError(t, auth.Reload())

	o := auth.Operators().Get("O")
	require.NoError(t, err)
	require.NotNil(t, o)

	a = o.Accounts().Get("A")
	require.NoError(t, err)
	require.NotNil(t, a)

	s = a.ScopedSigningKeys().GetScopeByRole("admin")
	require.NotNil(t, s)
	require.True(t, s.BearerToken())
}

func (suite *ProviderSuite) Test_ScopedPermissionsConnectionTypes() {
	t := suite.T()
	auth, _, a := setupTestWithOperatorAndAccount(suite)
	s, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	types := s.ConnectionTypes()
	require.NoError(t, types.Set("websocket"))
	require.Contains(t, types.Types(), "websocket")
	require.NoError(t, auth.Commit())

	require.NoError(t, auth.Reload())

	o := auth.Operators().Get("O")
	require.NoError(t, err)
	require.NotNil(t, o)

	a = o.Accounts().Get("A")
	require.NoError(t, err)
	require.NotNil(t, a)

	s = a.ScopedSigningKeys().GetScopeByRole("admin")
	require.NotNil(t, s)
	types = s.ConnectionTypes()
	require.Contains(t, types.Types(), "websocket")
}

func (suite *ProviderSuite) Test_ScopedPermissionsConnectionSources() {
	t := suite.T()
	auth, _, a := setupTestWithOperatorAndAccount(suite)
	s, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	sources := s.ConnectionSources()
	require.NoError(t, sources.Add("192.0.2.0/24"))
	require.Contains(t, sources.Sources(), "192.0.2.0/24")
	require.NoError(t, auth.Commit())

	require.NoError(t, auth.Reload())

	o := auth.Operators().Get("O")
	require.NoError(t, err)
	require.NotNil(t, o)

	a = o.Accounts().Get("A")
	require.NoError(t, err)
	require.NotNil(t, a)

	s = a.ScopedSigningKeys().GetScopeByRole("admin")
	require.NotNil(t, s)
	sources = s.ConnectionSources()
	require.Contains(t, sources.Sources(), "192.0.2.0/24")
}

func (suite *ProviderSuite) Test_ScopedPermissionsConnectionTimes() {
	t := suite.T()
	auth, _, a := setupTestWithOperatorAndAccount(suite)
	s, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	times := s.ConnectionTimes()
	require.NoError(t, times.Set(authb.TimeRange{Start: "08:00:00", End: "12:00:00"}))
	require.Len(t, times.List(), 1)
	require.NoError(t, auth.Commit())

	require.NoError(t, auth.Reload())

	o := auth.Operators().Get("O")
	require.NoError(t, err)
	require.NotNil(t, o)

	a = o.Accounts().Get("A")
	require.NoError(t, err)
	require.NotNil(t, a)

	s = a.ScopedSigningKeys().GetScopeByRole("admin")
	require.NotNil(t, s)
	times = s.ConnectionTimes()
	require.Len(t, times.List(), 1)
	require.Equal(t, times.List()[0].Start, "08:00:00")
	require.Equal(t, times.List()[0].End, "12:00:00")
}

func (suite *ProviderSuite) Test_ScopedPermissionsLocale() {
	t := suite.T()
	auth, _, a := setupTestWithOperatorAndAccount(suite)
	s, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	require.NoError(t, s.SetLocale("en_US"))
	require.NoError(t, auth.Commit())

	require.NoError(t, auth.Reload())

	o := auth.Operators().Get("O")
	require.NoError(t, err)
	require.NotNil(t, o)

	a = o.Accounts().Get("A")
	require.NoError(t, err)
	require.NotNil(t, a)

	s = a.ScopedSigningKeys().GetScopeByRole("admin")
	require.NotNil(t, s)
	require.Equal(t, "en_US", s.Locale())
}

func (suite *ProviderSuite) Test_ScopedPermissionsSubject() {
	t := suite.T()
	auth, _, a := setupTestWithOperatorAndAccount(suite)

	admin, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	require.NotNil(t, admin)

	pubPerms := admin.PubPermissions()
	require.NoError(t, pubPerms.SetAllow("foo", "bar"))
	require.NoError(t, pubPerms.SetDeny("baz"))

	subPerms := admin.SubPermissions()
	require.NoError(t, subPerms.SetAllow("foo", "bar"))
	require.NoError(t, subPerms.SetDeny("baz"))

	respPerms := admin.ResponsePermissions()
	require.NoError(t, respPerms.SetMaxMessages(10))
	require.NoError(t, respPerms.SetExpires(time.Second))

	require.NoError(t, auth.Reload())

	admin = a.ScopedSigningKeys().GetScopeByRole("admin")
	require.NotNil(t, admin)

	require.Contains(t, admin.PubPermissions().Allow(), "foo")
	require.Contains(t, admin.PubPermissions().Allow(), "bar")
	require.Contains(t, admin.PubPermissions().Deny(), "baz")

	require.Contains(t, admin.SubPermissions().Allow(), "foo")
	require.Contains(t, admin.SubPermissions().Allow(), "bar")
	require.Contains(t, admin.SubPermissions().Deny(), "baz")

	perm := admin.ResponsePermissions()
	require.NotNil(t, perm)
	require.Equal(t, 10, perm.MaxMessages())
	require.Equal(t, time.Second, perm.Expires())
}

func (suite *ProviderSuite) Test_ScopeRotation() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)

	scope, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	require.NotNil(t, scope)
	scope2, ok := a.ScopedSigningKeys().GetScope(scope.Key())
	require.True(t, ok)
	require.NotNil(t, scope2)

	key, err := a.ScopedSigningKeys().Rotate(scope.Key())
	require.NoError(t, err)
	require.NotEmpty(t, key)

	scope2, ok = a.ScopedSigningKeys().GetScope(scope.Key())
	require.False(t, ok)
	require.Nil(t, scope2)

	scope2, ok = a.ScopedSigningKeys().GetScope(key)
	require.True(t, ok)
	require.NotNil(t, scope2)

	ok, err = a.ScopedSigningKeys().Delete(key)
	require.NoError(t, err)
	require.True(t, ok)

	scope2, ok = a.ScopedSigningKeys().GetScope(key)
	require.False(t, ok)
	require.Nil(t, scope2)
}

func (suite *ProviderSuite) Test_SigningKeyRotation() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)

	sk, err := a.ScopedSigningKeys().Add()
	require.NoError(t, err)
	require.NotEmpty(t, sk)
	scope, ok := a.ScopedSigningKeys().GetScope(sk)
	require.True(t, ok)
	require.Nil(t, scope)

	u, err := a.Users().Add("U", sk)
	require.NoError(t, err)
	require.NotNil(t, u)

	require.Equal(t, sk, u.Issuer())

	key, err := a.ScopedSigningKeys().Rotate(sk)
	require.NoError(t, err)
	require.NotEmpty(t, key)

	require.Equal(t, key, u.Issuer())
}

func (suite *ProviderSuite) Test_AccountLimits() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)
	a, err := o.Accounts().Add("A")
	require.NoError(t, err)

	require.Equal(t, int64(-1), a.Limits().MaxData())
	require.Equal(t, int64(-1), a.Limits().MaxSubscriptions())
	require.Equal(t, int64(-1), a.Limits().MaxPayload())
	require.Equal(t, int64(-1), a.Limits().MaxConnections())
	require.Equal(t, int64(-1), a.Limits().MaxLeafNodeConnections())
	require.Equal(t, int64(-1), a.Limits().MaxImports())
	require.Equal(t, int64(-1), a.Limits().MaxExports())
	require.True(t, a.Limits().AllowWildcardExports())
	require.False(t, a.Limits().DisallowBearerTokens())

	require.NoError(t, a.Limits().SetMaxData(100))
	require.NoError(t, a.Limits().SetMaxSubscriptions(1_000))
	require.NoError(t, a.Limits().SetMaxPayload(1_0000))
	require.NoError(t, a.Limits().SetMaxConnections(3))
	require.NoError(t, a.Limits().SetMaxLeafNodeConnections(30))
	require.NoError(t, a.Limits().SetMaxImports(300))
	require.NoError(t, a.Limits().SetMaxExports(3_000))
	require.NoError(t, a.Limits().SetAllowWildcardExports(false))
	require.NoError(t, a.Limits().SetDisallowBearerTokens(true))
	require.NoError(t, auth.Commit())
	require.NoError(t, auth.Reload())

	require.Equal(t, int64(100), a.Limits().MaxData())
	require.Equal(t, int64(1_000), a.Limits().MaxSubscriptions())
	require.Equal(t, int64(10_000), a.Limits().MaxPayload())
	require.Equal(t, int64(3), a.Limits().MaxConnections())
	require.Equal(t, int64(30), a.Limits().MaxLeafNodeConnections())
	require.Equal(t, int64(300), a.Limits().MaxImports())
	require.Equal(t, int64(3_000), a.Limits().MaxExports())
	require.False(t, a.Limits().AllowWildcardExports())
	require.True(t, a.Limits().DisallowBearerTokens())
}

func (suite *ProviderSuite) testTier(auth authb.Auth, account authb.Account, tier int8) {
	t := suite.T()
	var err error

	js := account.Limits().JetStream()
	require.False(t, js.IsJetStreamEnabled())
	lim, err := js.Get(tier)
	require.NoError(t, err)
	if tier == 0 {
		require.NotNil(t, lim)
	} else {
		require.Nil(t, lim)
		lim, err = js.Add(tier)
		require.NoError(t, err)
	}
	ok, err := lim.IsUnlimited()
	require.NoError(t, err)
	require.False(t, ok)

	num, err := lim.MaxMemoryStorage()
	require.NoError(t, err)
	require.Equal(t, int64(0), num)

	num, err = lim.MaxDiskStorage()
	require.NoError(t, err)
	require.Equal(t, int64(0), num)

	num, err = lim.MaxMemoryStreamSize()
	require.NoError(t, err)
	require.Equal(t, int64(0), num)

	num, err = lim.MaxDiskStreamSize()
	require.NoError(t, err)
	require.Equal(t, int64(0), num)

	tf, err := lim.MaxStreamSizeRequired()
	require.NoError(t, err)
	require.False(t, tf)

	num, err = lim.MaxStreams()
	require.NoError(t, err)
	require.Equal(t, int64(0), num)

	num, err = lim.MaxConsumers()
	require.NoError(t, err)
	require.Equal(t, int64(0), num)

	num, err = lim.MaxAckPending()
	require.NoError(t, err)
	require.Equal(t, int64(0), num)

	require.NoError(t, lim.SetMaxDiskStorage(1000))
	require.NoError(t, lim.SetMaxMemoryStorage(2000))
	require.NoError(t, lim.SetMaxMemoryStreamSize(4000))
	require.NoError(t, lim.SetMaxDiskStreamSize(8000))
	require.NoError(t, lim.SetMaxStreamSizeRequired(true))
	require.NoError(t, lim.SetMaxStreams(5))
	require.NoError(t, lim.SetMaxConsumers(50))
	require.NoError(t, lim.SetMaxAckPending(22))

	tf, err = lim.IsUnlimited()
	require.NoError(t, err)
	require.False(t, tf)

	require.NoError(t, auth.Commit())
	require.NoError(t, auth.Reload())

	lim, err = js.Get(tier)
	require.NoError(t, err)
	require.NotNil(t, lim)

	tf, err = lim.IsUnlimited()
	require.NoError(t, err)
	require.False(t, tf)

	num, err = lim.MaxDiskStorage()
	require.NoError(t, err)
	require.Equal(t, int64(1000), num)

	num, err = lim.MaxMemoryStorage()
	require.NoError(t, err)
	require.Equal(t, int64(2000), num)

	num, err = lim.MaxMemoryStreamSize()
	require.NoError(t, err)
	require.Equal(t, int64(4000), num)

	num, err = lim.MaxDiskStreamSize()
	require.NoError(t, err)
	require.Equal(t, int64(8000), num)

	tf, err = lim.MaxStreamSizeRequired()
	require.NoError(t, err)
	require.True(t, tf)

	num, err = lim.MaxStreams()
	require.NoError(t, err)
	require.Equal(t, int64(5), num)

	num, err = lim.MaxConsumers()
	require.NoError(t, err)
	require.Equal(t, int64(50), num)

	num, err = lim.MaxAckPending()
	require.NoError(t, err)
	require.Equal(t, int64(22), num)

	require.NoError(t, lim.SetUnlimited())
	tf, err = lim.IsUnlimited()
	require.NoError(t, err)
	require.True(t, tf)

}

func (suite *ProviderSuite) Test_AccountJetStreamLimits() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)
	o, err := auth.Operators().Add("O")
	require.NoError(t, err)
	a, err := o.Accounts().Add("A")
	require.NoError(t, err)
	suite.testTier(auth, a, 0)
	b, err := o.Accounts().Add("B")
	require.NoError(t, err)
	suite.testTier(auth, b, 1)
}

func (suite *ProviderSuite) Test_AccountSkUpdate() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)

	operators := auth.Operators()
	require.Empty(t, operators.List())

	o, err := operators.Add("O")
	require.NoError(t, err)
	require.NotNil(t, o)

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)
	require.NotNil(t, a)

	require.NoError(t, auth.Commit())
	require.NoError(t, auth.Reload())

	o = operators.Get("O")
	require.NotNil(t, o)

	a = o.Accounts().Get("A")
	require.NotNil(t, a)

	k, err := a.ScopedSigningKeys().Add()
	require.NoError(t, err)
	require.NotEmpty(t, k)

	require.NoError(t, auth.Commit())
	require.NoError(t, auth.Reload())

	o = operators.Get("O")
	require.NotNil(t, o)
	a = o.Accounts().Get("A")
	require.NotNil(t, a)
	scope, ok := a.ScopedSigningKeys().GetScope(k)
	require.Nil(t, scope)
	require.True(t, ok)
}

func (suite *ProviderSuite) Test_AccountSigningKeys() {
	t := suite.T()
	auth, err := authb.NewAuth(suite.Provider)
	require.NoError(t, err)

	operators := auth.Operators()
	require.Empty(t, operators.List())

	a := suite.MaybeCreate(auth, "O", "A")
	require.NotNil(t, a)

	var keys []string
	k, err := a.ScopedSigningKeys().Add()
	require.NoError(t, err)
	require.NotEmpty(t, k)
	keys = append(keys, k)

	sl, err := a.ScopedSigningKeys().AddScope("admin")
	require.NoError(t, err)
	require.NotNil(t, sl)
	keys = append(keys, sl.Key())
	keys2 := a.ScopedSigningKeys().List()
	for _, k := range keys {
		require.Contains(t, keys2, k)
	}

	roles := a.ScopedSigningKeys().ListRoles()
	t.Log(roles)
	require.NotNil(t, roles)
	require.Len(t, roles, 1)
	require.Contains(t, roles, "admin")
}

func (s *ProviderSuite) Test_ServiceRequiresName() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	operators := auth.Operators()
	s.Empty(operators.List())

	a := s.MaybeCreate(auth, "O", "A")
	s.NotNil(a)

	_, err = a.Exports().AddService("", "q.foo.>")
	s.Error(err)
}

func (s *ProviderSuite) Test_ServiceRequiresSubject() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	operators := auth.Operators()
	s.Empty(operators.List())

	o, err := operators.Add("O")
	s.NoError(err)
	s.NotNil(o)

	a, err := o.Accounts().Add("A")
	s.NoError(err)
	s.NotNil(a)

	_, err = a.Exports().AddService("name", "")
	s.Error(err)
}

func (s *ProviderSuite) Test_AddService() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	operators := auth.Operators()
	s.Empty(operators.List())

	o, err := operators.Add("O")
	s.NoError(err)
	s.NotNil(o)

	a, err := o.Accounts().Add("A")
	s.NoError(err)
	s.NotNil(a)

	service, err := authb.NewService("q", "q.*")
	s.NoError(err)

	err = a.Exports().AddServiceWithConfig(service)
	s.NoError(err)

	service = a.Exports().GetService("q.*")
	s.NotNil(service)
	s.Equal("q", service.Name())
	s.Equal("q.*", service.Subject())
}

func (s *ProviderSuite) Test_ServiceCrud() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	a := s.MaybeCreate(auth, "O", "A")
	s.Len(a.Exports().Services(), 0)

	service, err := a.Exports().AddService("foos", "q.foo.>")
	s.NoError(err)
	s.NotNil(service)
	s.Equal("foos", service.Name())
	s.Equal("q.foo.>", service.Subject())
	s.NoError(service.SetTokenRequired(true))
	s.Equal(true, service.TokenRequired())

	s.NoError(auth.Commit())
	s.NoError(auth.Reload())

	a = s.GetAccount(auth, "O", "A")
	s.NotNil(a)

	services := a.Exports().Services()
	s.Len(services, 1)
	s.Equal("foos", services[0].Name())
	s.Equal("q.foo.>", services[0].Subject())
	s.Equal(true, services[0].TokenRequired())

	s.Nil(a.Exports().GetServiceByName("foo"))

	service = a.Exports().GetServiceByName("foos")
	s.NotNil(service)

	service = a.Exports().GetService("q.foo.>")
	s.NotNil(service)

	s.NoError(service.SetName("bar"))
	s.NoError(service.SetTokenRequired(false))
	s.NoError(service.SetSubject("bar.*"))
	s.NoError(auth.Commit())
	s.NoError(auth.Reload())

	services = a.Exports().Services()
	s.Len(services, 1)
	s.Equal("bar", services[0].Name())
	s.Equal("bar.*", services[0].Subject())
	s.Equal(false, services[0].TokenRequired())
}

func (s *ProviderSuite) Test_StreamCrud() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	a := s.MaybeCreate(auth, "O", "A")
	s.Len(a.Exports().Streams(), 0)

	stream, err := a.Exports().AddStream("foos", "q.foo.>")
	s.NoError(err)
	s.NotNil(stream)
	s.Equal("foos", stream.Name())
	s.Equal("q.foo.>", stream.Subject())
	s.NoError(stream.SetTokenRequired(true))
	s.Equal(true, stream.TokenRequired())

	s.NoError(auth.Commit())
	s.NoError(auth.Reload())

	a = s.GetAccount(auth, "O", "A")
	s.NotNil(a)

	streams := a.Exports().Streams()
	s.Len(streams, 1)
	s.Equal("foos", streams[0].Name())
	s.Equal("q.foo.>", streams[0].Subject())
	s.Equal(true, streams[0].TokenRequired())

	s.Nil(a.Exports().GetStreamByName("foo"))

	stream = a.Exports().GetStreamByName("foos")
	s.NotNil(stream)

	stream = a.Exports().GetStream("q.foo.>")
	s.NotNil(stream)

	s.NoError(stream.SetName("bar"))
	s.NoError(stream.SetTokenRequired(false))
	s.NoError(stream.SetSubject("bar.*"))
	s.NoError(auth.Commit())
	s.NoError(auth.Reload())

	streams = a.Exports().Streams()
	s.Len(streams, 1)
	s.Equal("bar", streams[0].Name())
	s.Equal("bar.*", streams[0].Subject())
	s.Equal(false, streams[0].TokenRequired())
}

func (s *ProviderSuite) Test_SetStream() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	a := s.MaybeCreate(auth, "O", "A")
	s.Len(a.Exports().Streams(), 0)

	_, err = a.Exports().AddService("a", "q.a.>")
	s.NoError(err)

	_, err = a.Exports().AddStream("a", "a.>")
	s.NoError(err)

	// empty set clears
	err = a.Exports().SetStreams(nil)
	s.NoError(err)
	s.Len(a.Exports().Streams(), 0)
	s.Len(a.Exports().Services(), 1)

	service1, err := authb.NewService("q", "q")
	s.NoError(err)
	service2, err := authb.NewService("qq", "qq")
	s.NoError(err)

	err = a.Exports().SetServices(service1, service2)
	s.NoError(err)
	s.Nil(a.Exports().GetService("q.a.>"))
	services := a.Exports().Services()
	s.Len(services, 2)
}

func (s *ProviderSuite) Test_ServiceRevocationCrud() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	a := s.MaybeCreate(auth, "O", "A")
	s.NotNil(a)
	s.Len(a.Exports().Services(), 0)

	service, err := a.Exports().AddService("foos", "q.foo.>")
	s.NoError(err)
	s.NotNil(service)
	s.Equal("foos", service.Name())
	s.Equal("q.foo.>", service.Subject())

	// Let's create a revocation for an account
	k, _ := authb.KeyFor(nkeys.PrefixByteAccount)

	// Since the export is public this fails
	err = service.Revocations().Add(k.Public, time.Now())
	s.Error(err)
	s.True(errors.Is(err, authb.ErrRevocationPublicExportsNotAllowed))

	// Require a token, and revocation is now added
	err = service.SetTokenRequired(true)
	s.NoError(err)
	err = service.Revocations().Add(k.Public, time.Now())
	s.NoError(err)

	s.NoError(auth.Commit())
	s.NoError(auth.Reload())

	// reload the configuration, find the service
	a = s.GetAccount(auth, "O", "A")
	s.NotNil(a)
	service = a.Exports().GetServiceByName("foos")
	s.NotNil(service)

	revocations := service.Revocations()

	// check the revocation is there
	s.Len(revocations.List(), 1)
	tf, err := revocations.HasRevocation("*")
	s.Nil(err)
	s.False(tf)

	// try a key that is not supported
	uk, _ := authb.KeyFor(nkeys.PrefixByteUser)
	tf, err = revocations.HasRevocation(uk.Public)
	s.Error(err)
	s.False(tf)

	// find the key we want
	tf, err = revocations.HasRevocation(k.Public)
	s.NoError(err)
	s.True(tf)

	// test listing
	entries := revocations.List()
	s.Len(entries, 1)
	s.Equal(k.Public, entries[0].PublicKey())

	// try to remove it - it doesn't exist
	ok, err := revocations.Delete("*")
	s.NoError(err)
	s.False(ok)

	// add it
	s.NoError(revocations.Add("*", time.Now()))
	entries = revocations.List()
	s.Len(entries, 2)

	tf, _ = revocations.HasRevocation(k.Public)
	s.True(tf)
	tf, _ = revocations.HasRevocation("*")
	s.True(tf)

	// verify the list contains them
	var wildcard authb.RevocationEntry
	var account authb.RevocationEntry
	for _, e := range entries {
		if e.PublicKey() == "*" {
			wildcard = e
		} else {
			account = e
		}
	}
	s.NotNil(wildcard)
	s.NotNil(account)

	tf, err = revocations.Delete(k.Public)
	s.NoError(err)
	s.True(tf)

	entries = revocations.List()
	s.Len(entries, 1)
	tf, _ = revocations.HasRevocation(k.Public)
	s.False(tf)

	// add them both
	s.NoError(revocations.SetRevocations([]authb.RevocationEntry{account, wildcard}))
	entries = revocations.List()
	s.Len(entries, 2)

	// clear
	s.NoError(revocations.SetRevocations(nil))
	entries = revocations.List()
	s.Len(entries, 0)

	// yesterday
	s.NoError(revocations.Add(k.Public, time.Now().Add(time.Hour*-24)))

	// add a wildcard as of now (includes and rejects the previous revocation
	s.NoError(revocations.Add("*", time.Now()))
	entries = revocations.List()
	s.Len(entries, 2)

	// wildcard includes yesterday
	removed, err := revocations.Compact()
	s.NoError(err)
	s.Len(removed, 1)
	s.Equal(k.Public, removed[0].PublicKey())
}

func (s *ProviderSuite) Test_AccountRevocationEmpty() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	a := s.MaybeCreate(auth, "O", "A")
	s.NotNil(a)
	r := a.Revocations()
	s.NotNil(r)
	s.Len(r.List(), 0)
}

func (s *ProviderSuite) Test_AccountRevokesRejectNonUserKey() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	a := s.MaybeCreate(auth, "O", "A")
	s.NotNil(a)
	revocations := a.Revocations()
	s.NotNil(revocations)
	s.Len(revocations.List(), 0)

	err = revocations.Add(s.AccountKey().Public, time.Now())
	s.Error(err)
}

func (s *ProviderSuite) Test_AccountRevokeUser() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	a := s.MaybeCreate(auth, "O", "A")
	s.NotNil(a)
	revocations := a.Revocations()
	s.NotNil(revocations)
	s.Len(revocations.List(), 0)

	uk := s.UserKey().Public
	err = revocations.Add(uk, time.Now())
	s.NoError(err)

	revokes := revocations.List()
	s.Len(revokes, 1)
	s.Equal(uk, revokes[0].PublicKey())

	s.NoError(auth.Commit())
	s.NoError(auth.Reload())

	a = s.GetAccount(auth, "O", "A")
	s.NotNil(a)
	s.True(a.Revocations().HasRevocation(uk))

	ok, err := a.Revocations().Delete(uk)
	s.NoError(err)
	s.True(ok)

	s.NoError(auth.Commit())
	s.NoError(auth.Reload())

	a = s.GetAccount(auth, "O", "A")
	s.NotNil(a)
	s.False(a.Revocations().HasRevocation(uk))
}

func (s *ProviderSuite) Test_AccountRevokeWildcard() {
	auth, err := authb.NewAuth(s.Provider)
	s.NoError(err)

	a := s.MaybeCreate(auth, "O", "A")
	s.NotNil(a)
	revocations := a.Revocations()
	s.NotNil(revocations)
	s.Len(revocations.List(), 0)

	err = revocations.Add("*", time.Now())
	s.NoError(err)

	revokes := revocations.List()
	s.Len(revokes, 1)
	s.Equal("*", revokes[0].PublicKey())
}
