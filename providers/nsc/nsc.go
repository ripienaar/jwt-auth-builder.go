package nsc

import (
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nsc/v2/cmd/store"
	"github.com/nats-io/nsc/v2/home"
	"github.com/synadia-io/jwt-auth-builder.go"

	"os"
	"path/filepath"
)

// NscProvider is an AuthProvider that stores data using the nsc Store.
type NscProvider struct {
	storesDir string
	keysDir   string
}

func NewNscProvider(storesDir string, keysDir string) *NscProvider {
	if storesDir == "" {
		storesDir = home.NscDataHome(home.StoresSubDirName)
	}
	if keysDir == "" {
		keysDir = home.NscDataHome(home.KeysSubDirName)
	}
	store.KeyStorePath = keysDir
	return &NscProvider{storesDir: storesDir, keysDir: keysDir}
}

func (a *NscProvider) MaybeMakeDir(path string) error {
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(path, 0700)
	}
	return err
}

func (a *NscProvider) Load() ([]*authb.OperatorData, error) {
	var operators []*authb.OperatorData
	if err := a.MaybeMakeDir(a.storesDir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(a.storesDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			si, err := a.loadStore(e.Name())
			if err != nil {
				return nil, err
			}
			od, err := a.loadOperator(si)
			if err != nil {
				return nil, err
			}
			operators = append(operators, od)
		}
	}
	return operators, nil
}

func (a *NscProvider) loadStore(name string) (store.IStore, error) {
	fi, err := os.Stat(filepath.Join(a.storesDir, name, store.NSCFile))
	if err == nil && fi.Size() > 0 {
		s, err := store.LoadStore(filepath.Join(a.storesDir, name))
		if err != nil {
			return nil, err
		}
		return s, nil

	}
	return nil, err
}

func (a *NscProvider) loadOperator(si store.IStore) (*authb.OperatorData, error) {
	token, err := si.ReadRawOperatorClaim()
	if err != nil {
		return nil, err
	}
	oc, err := jwt.DecodeOperatorClaims(string(token))
	if err != nil {
		return nil, err
	}
	od := &authb.OperatorData{BaseData: authb.BaseData{EntityName: si.GetName(), Loaded: oc.IssuedAt}, Claim: oc}
	ks := store.NewKeyStore(od.EntityName)
	kp, err := ks.GetKeyPair(oc.Issuer)
	if err != nil {
		return nil, err
	}
	if kp != nil {
		od.Key, _ = authb.KeyFromNkey(kp, nkeys.PrefixByteOperator)
	}
	if len(oc.SigningKeys) > 0 {
		for _, sk := range oc.SigningKeys {
			skp, _ := ks.GetKeyPair(sk)
			if skp != nil {
				k, _ := authb.KeyFromNkey(skp, nkeys.PrefixByteOperator)
				if k != nil {
					od.OperatorSigningKeys = append(od.OperatorSigningKeys, k)
				}
			}
		}
	}
	od.AccountDatas, err = a.loadAccounts(si, ks)
	if err != nil {
		return nil, err
	}
	for _, ad := range od.AccountDatas {
		ad.Operator = od
	}
	return od, err
}

func (a *NscProvider) loadAccounts(si store.IStore, ks store.KeyStore) ([]*authb.AccountData, error) {
	var datas []*authb.AccountData
	accountNames, err := si.ListSubContainers(store.Accounts)
	if err != nil {
		return nil, err
	}
	for _, name := range accountNames {
		data, err := a.loadAccount(si, ks, name)
		if err != nil {
			return nil, err
		}
		datas = append(datas, data)
	}
	return datas, nil
}

func (a *NscProvider) loadAccount(si store.IStore, ks store.KeyStore, name string) (*authb.AccountData, error) {
	ad := &authb.AccountData{BaseData: authb.BaseData{EntityName: name}}
	token, err := si.ReadRawAccountClaim(name)
	if err != nil {
		return nil, err
	}
	ad.Token = string(token)
	ad.Claim, err = jwt.DecodeAccountClaims(ad.Token)
	if err != nil {
		return nil, err
	}
	ad.Loaded = ad.Claim.IssuedAt
	k, _ := ks.GetKeyPair(ad.Claim.Subject)
	if k != nil {
		ad.Key, _ = authb.KeyFromNkey(k, nkeys.PrefixByteAccount)
	}
	keys := ad.Claim.SigningKeys.Keys()
	for _, k := range keys {
		skp, _ := ks.GetKeyPair(k)
		if skp != nil {
			sk, _ := authb.KeyFromNkey(skp, nkeys.PrefixByteOperator)
			if sk != nil {
				ad.AccountSigningKeys = append(ad.AccountSigningKeys, sk)
			}
		}
	}

	ad.UserDatas, err = a.loadUsers(si, ks, name)
	if err != nil {
		return nil, err
	}
	for _, u := range ad.UserDatas {
		u.AccountData = ad
		u.RejectEdits = u.IsScoped()
	}

	return ad, err
}

func (a *NscProvider) loadUsers(si store.IStore, ks store.KeyStore, account string) ([]*authb.UserData, error) {
	var datas []*authb.UserData
	names, err := si.ListEntries(store.Accounts, account, store.Users)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		data, err := a.loadUser(si, ks, account, name)
		if err != nil {
			return nil, err
		}
		datas = append(datas, data)
	}
	return datas, nil
}

func (a *NscProvider) loadUser(si store.IStore, ks store.KeyStore, account string, name string) (*authb.UserData, error) {
	var err error
	ud := &authb.UserData{BaseData: authb.BaseData{EntityName: name}}
	token, err := si.ReadRawUserClaim(account, name)
	if err != nil {
		return nil, err
	}
	ud.Token = string(token)
	ud.Claim, err = jwt.DecodeUserClaims(ud.Token)
	if err != nil {
		return nil, err
	}
	ud.Loaded = ud.Claim.IssuedAt
	kp, err := ks.GetKeyPair(ud.Claim.Subject)
	if err != nil {
		return nil, err
	}
	ud.Key, err = authb.KeyFromNkey(kp, nkeys.PrefixByteUser)
	if err != nil {
		return nil, err
	}
	return ud, nil
}

func (a *NscProvider) Store(operators []*authb.OperatorData) error {
	for _, o := range operators {
		var err error
		ks := store.NewKeyStore(o.EntityName)

		if o.Loaded == 0 {
			nk := &store.NamedKey{Name: o.EntityName, KP: o.Key.Pair}
			_, err = store.CreateStore("", a.storesDir, nk)
			if err != nil {
				return err
			}
			_, err = ks.Store(o.Key.Pair)
			if err != nil {
				return err
			}
		}
		s, err := a.loadStore(o.EntityName)
		if err != nil {
			return err
		}
		// if the operator changed configuration save it
		if o.Claim.IssuedAt > o.Loaded {
			if err := s.StoreRaw([]byte(o.Token)); err != nil {
				return err
			}
		}
		// this will save all keys that were added, operator, account, users..
		for _, k := range o.AddedKeys {
			_, err := ks.Store(k.Pair)
			if err != nil {
				return err
			}
		}
		o.AddedKeys = nil
		// this will remove all keys that were added, operator, account, users..
		for _, k := range o.DeletedKeys {
			err := ks.Remove(k)
			if err != nil {
				return err
			}
		}
		o.DeletedKeys = nil

		for _, account := range o.AccountDatas {
			if account.Claim.IssuedAt > account.Loaded {
				if err := s.StoreRaw([]byte(account.Token)); err != nil {
					return err
				}
				// check that signing keys were not modified
				account.Loaded = account.Claim.IssuedAt

				for _, u := range account.UserDatas {
					if u.Claim.IssuedAt > u.Loaded {
						if err := s.StoreRaw([]byte(u.Token)); err != nil {
							return err
						}
						u.Loaded = u.Claim.IssuedAt
					}
				}
			}
			for _, u := range account.DeletedUsers {
				if err := s.Delete(store.Accounts, account.EntityName, store.Users, store.JwtName(u.EntityName)); err != nil {
					return err
				}
			}
		}
		// update the loaded so that other mods can be detected
		o.Loaded = o.Claim.IssuedAt
	}
	return nil
}
