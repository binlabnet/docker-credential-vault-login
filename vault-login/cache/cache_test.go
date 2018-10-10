package cache

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/awstesting"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/helper/jsonutil"
	"github.com/morningconsult/docker-credential-vault-login/vault-login/cache/logging"
	"github.com/morningconsult/docker-credential-vault-login/vault-login/config"
	test "github.com/morningconsult/docker-credential-vault-login/vault-login/testing"
)

func TestNewCacheUtil(t *testing.T) {
	const cacheDir = "testdata"

	os.Setenv(EnvCacheDir, cacheDir)

	cases := []struct {
		name      string
		env       string
		cacheType string
	}{
		{
			"enabled-a",
			"false",
			"default",
		},
		{
			"enabled-b",
			"f",
			"default",
		},
		{
			"enabled-c",
			"i am not a bool",
			"default",
		},
		{
			"enabled-d",
			"",
			"default",
		},
		{
			"disabled-a",
			"true",
			"null",
		},
		{
			"disabled-b",
			"t",
			"null",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv(EnvDisableCache, tc.env)
			cacheUtilUntyped := NewCacheUtil(nil)

			switch tc.cacheType {
			case "default":
				if _, ok := cacheUtilUntyped.(*DefaultCacheUtil); !ok {
					t.Fatalf("Expected to receive an instance of cache.DefaultCacheUtil but didn't")
				}
			case "null":
				if _, ok := cacheUtilUntyped.(*NullCacheUtil); !ok {
					t.Fatalf("Expected to receive an instance of cache.DefaultCacheUtil but didn't")
				}
			default:
				t.Fatalf("Received unknown CacheUtil type: %T", cacheUtilUntyped)
			}
		})
	}
}

func TestNewCacheUtil_BackupCache(t *testing.T) {
	// This will cause github.com/mitchellh/go-homedir.Expand() to fail
	env := awstesting.StashEnv()
	defer awstesting.PopEnv(env)

	cacheUtil := NewCacheUtil(nil)

	if cacheUtil.GetCacheDir() != BackupCacheDir {
		t.Fatalf("expected CacheUtil.cacheDir to be %q, but got %q instead",
			BackupCacheDir, cacheUtil.GetCacheDir())
	}
}

func TestDefaultCacheUtil_GetCacheDir(t *testing.T) {
	const cacheDir = "testdata"

	os.Unsetenv(EnvDisableCache)
	os.Setenv(EnvCacheDir, cacheDir)

	cacheUtil := NewDefaultCacheUtil(nil)
	if cacheUtil.GetCacheDir() != cacheDir {
		t.Fatalf("Expected cacheUtil.cacheDir to be %q, but got %q instead",
			cacheDir, cacheUtil.cacheDir)
	}
}

// func TestDefaultCacheUtil_GetCachedToken_ClearsTokens(t *testing.T) {
// 	const cacheDir = "testdata"
// 	const method = config.VaultAuthMethodAWSIAM

// 	os.Unsetenv(EnvDisableCache)
// 	os.Unsetenv(EnvCipherKey)
// 	os.Setenv(EnvCacheDir, cacheDir)

// 	cacheUtil := NewDefaultCacheUtil(nil)
// 	cacheUtil.ClearCachedToken(method)
// 	defer cacheUtil.ClearCachedToken(method)

// 	cases := []struct {
// 		name      string
// 		tokenJSON map[string]interface{}
// 	}{
// 		{
// 			"expired",
// 			map[string]interface{}{
// 				"token":      "token!",
// 				"expiration": time.Now().Add(-10 * time.Hour).Unix(),
// 				"renewable":  false,
// 			},
// 		},
// 		{
// 			"lookup-fails",
// 			map[string]interface{}{
// 				"token":      "token!",
// 				"expiration": "not an int",
// 				"renewable":  false,
// 			},
// 		},
// 		{
// 			"renew-fails",
// 			map[string]interface{}{
// 				"token":      "token!",
// 				"expiration": time.Now().Add(time.Second * time.Duration(GracePeriodSeconds/2)).Unix(),
// 				"renewable":  true,
// 			},
// 		},
// 	}

// 	for _, tc := range cases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			cacheUtil.ClearCachedToken(method)
// 			defer cacheUtil.ClearCachedToken(method)
// 			writeJSONToFile(t, tc.tokenJSON, cacheUtil.TokenFilename(method))

// 			if tc.name == "renew-fails" {
// 				// This will trigger an error when
// 				// github.com/hasicorp/vault/api.Client.NewClient()
// 				// is called
// 				os.Setenv(api.EnvRateLimit, "not an int!")
// 				defer os.Unsetenv(api.EnvRateLimit)
// 			}

// 			token := cacheUtil.GetCachedToken(method)
// 			if token != "" {
// 				t.Fatal("returned token should be an empty string")
// 			}

// 			files, err := filepath.Glob(cacheUtil.basename(method) + "*")
// 			if err != nil {
// 				t.Fatal(err)
// 			}

// 			if len(files) > 0 {
// 				t.Fatal("GetCachedToken() should have cleared all tokens")
// 			}
// 		})
// 	}
// }

func TestDefaultCacheUtil_CacheNewToken(t *testing.T) {
	const roleName = "dev-test"

	os.Unsetenv(EnvDisableCache)
	os.Setenv(EnvCacheDir, "testdata")
	os.Unsetenv(EnvCipherKey)

	cacheUtil := NewDefaultCacheUtil(nil)

	// Setup some test values
	method := config.VaultAuthMethodAWSIAM
	goodExpiration := time.Now().Add(time.Minute * 20).Unix()
	token, err := uuid.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		arg  interface{}
		err  bool
	}{
		{
			"with-cached-token",
			&CachedToken{
				Token:      token,
				Expiration: goodExpiration,
				Renewable:  true,
			},
			false,
		},
		{
			"with-secret",
			&api.Secret{
				Auth: &api.SecretAuth{
					ClientToken:   token,
					Renewable:     true,
					LeaseDuration: 86400,
				},
			},
			false,
		},
		{
			"secret-bad-token",
			&api.Secret{
				Data: map[string]interface{}{
					// Token is not a string
					"id": 1234,
				},
			},
			true,
		},
		{
			"secret-bad-ttl",
			&api.Secret{
				Data: map[string]interface{}{
					// Token is not a string
					"ttl": "I really should be an int",
				},
			},
			true,
		},
		{
			"secret-bad-renewable",
			&api.Secret{
				Data: map[string]interface{}{
					// Token is not a string
					"renewable": "I should really be a boolean",
				},
			},
			true,
		},
		{
			"unsupported-type",
			"i'm just a string",
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cacheUtil.ClearCachedToken(method)
			err := cacheUtil.CacheNewToken(tc.arg, method)
			defer cacheUtil.ClearCachedToken(method)

			if tc.err {
				if err == nil {
					t.Fatal("expected an error but didn't receive one")
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error but received one: %v", err)
			}

			cachedToken := loadTokenFromFile(t, cacheUtil.TokenFilename(method))
			if cachedToken.Token != token {
				t.Fatalf("expected token %q but got %q instead", token, cachedToken.Token)
			}
			if cachedToken.Expiration == 0 {
				t.Fatal("expected token to have an expiration date, but it didn't")
			}
		})
	}
}

func TestDefaultCacheUtil_LookupToken(t *testing.T) {
	const roleName = "dev-test"

	os.Unsetenv(EnvDisableCache)
	os.Setenv(EnvCacheDir, "testdata")
	os.Unsetenv(EnvCipherKey)

	cacheUtil := NewDefaultCacheUtil(nil)

	// Setup some test values
	method := config.VaultAuthMethodAWSIAM
	goodExpiration := time.Now().Add(time.Minute * 20).Unix()
	token, err := uuid.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		token      string
		expiration interface{}
		renewable  interface{}
		err        bool
	}{
		{
			"success",
			token,
			goodExpiration,
			true,
			false,
		},
		{
			"non-int-expiration",
			token,
			"i am not an int!",
			true,
			true,
		},
		{
			"empty-token",
			"",
			goodExpiration,
			true,
			true,
		},
		{
			"file-doesnt-exist",
			token,
			goodExpiration,
			true,
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cacheUtil.ClearCachedToken(method)

			// This represents a CachedToken instance. An actual
			// CachedToken object is not created so that mismatched
			// data types can be tested
			json := map[string]interface{}{
				"token":      tc.token,
				"expiration": tc.expiration,
				"renewable":  tc.renewable,
			}

			if tc.name != "file-doesnt-exist" {
				writeJSONToFile(t, json, cacheUtil.TokenFilename(method))
			}

			// Delete the file at the end of the test
			defer cacheUtil.ClearCachedToken(method)

			_, err := cacheUtil.LookupToken(method)
			if tc.err && (err == nil) {
				t.Fatal("expected an error but didn't receive one")
			}

			if !tc.err && (err != nil) {
				t.Fatalf("expected no error but received one: %v", err)
			}
		})
	}
}

func TestDefaultCacheUtil_RenewToken(t *testing.T) {
	const roleName = "dev-test"

	os.Unsetenv(EnvDisableCache)
	os.Setenv(EnvCacheDir, "testdata")
	os.Unsetenv(EnvCipherKey)

	// Start the Vault testing cluster
	cluster := test.StartTestCluster(t)
	defer cluster.Cleanup()

	client := test.NewPreConfiguredVaultClient(t, cluster)
	rootToken := client.Token()

	cases := []struct {
		name      string
		renewable bool
		ttl       string
		method    config.VaultAuthMethod
		err       bool
	}{
		{
			"renewable",
			true,
			"1h",
			config.VaultAuthMethodAWSIAM,
			false,
		},
		{
			"non-renewable",
			false,
			"1h",
			config.VaultAuthMethodAWSIAM,
			true,
		},
		{
			"new-vault-client-error",
			true,
			"1h",
			config.VaultAuthMethodAWSIAM,
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client.SetToken(rootToken)

			var cacheUtil *DefaultCacheUtil
			if tc.name == "new-vault-client-error" {
				// This will trigger an error
				os.Setenv(api.EnvRateLimit, "not an int!")
				defer os.Unsetenv(api.EnvRateLimit)
				cacheUtil = NewDefaultCacheUtil(nil)
			} else {
				cacheUtil = NewDefaultCacheUtil(client)
			}

			// Create a token
			secret, err := client.Logical().Write(filepath.Join("auth", "token", "create"), map[string]interface{}{
				"renewable": tc.renewable,
				"ttl":       tc.ttl,
				"policies":  []string{"test"},
			})
			if err != nil {
				t.Fatal(err)
			}

			token, err := secret.TokenID()
			if err != nil {
				t.Fatal(err)
			}

			err = cacheUtil.RenewToken(&CachedToken{
				Token:      token,
				Expiration: time.Now().Add(time.Hour * 1).Unix(),
				Renewable:  tc.renewable,
				AuthMethod: tc.method,
			})

			cacheUtil.ClearCachedToken(tc.method)

			if tc.err && (err == nil) {
				t.Fatal("expected an error but didn't receive one")
			}

			if !tc.err && (err != nil) {
				t.Fatalf("expected no error but received one: %v", err)
			}
		})
	}
}

func TestDefaultCacheUtil_GetEncryptedToken(t *testing.T) {
	const (
		roleName  = "dev-test"
		cipherKey = "hello darkness my old friend ive"
		method    = config.VaultAuthMethodAWSIAM
		token     = "random token"
	)

	var expiration = time.Now().Unix()

	os.Unsetenv(EnvDisableCache)
	os.Setenv(EnvCacheDir, "testdata")

	cases := []struct {
		name string
		key  string
		err  bool
	}{
		{
			"decrypts",
			cipherKey,
			false,
		},
		{
			"wrong-key",
			"asdfasdfasdfasdfasdfasdfasdfasdfasdf",
			true,
		},
		{
			"malformed-file",
			cipherKey,
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv(EnvCipherKey, tc.key)
			defer os.Unsetenv(EnvCipherKey)

			cacheUtil := NewDefaultCacheUtil(nil)

			cacheUtil.ClearCachedToken(method)
			defer cacheUtil.ClearCachedToken(method)

			if tc.name == "malformed-file" {
				writeDataToFile(t, []byte(""), cacheUtil.TokenFilename(method))
			} else {
				json := map[string]interface{}{
					"token":      token,
					"expiration": expiration,
				}
				encryptJSONAndWriteFile(t, json, cacheUtil.TokenFilename(method), cipherKey)
			}

			cachedToken, err := cacheUtil.LookupToken(method)
			if tc.err {
				if err == nil {
					t.Fatal("expected an error but didn't receive one")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cachedToken.Token != token {
				t.Fatalf("expected cached token ID of %q but got %q", token, cachedToken.Token)
			}
			if cachedToken.Expiration != expiration {
				t.Fatalf("expected cached token expiration date of %d but got %d instead",
					expiration, cachedToken.Expiration)
			}
		})
	}
}

func TestDefaultCacheUtil_EncryptAndCacheToken(t *testing.T) {
	const (
		roleName  = "dev-test"
		cipherKey = "hello darkness my old friend ive"
		method    = config.VaultAuthMethodAWSIAM
		token     = "random token"
	)

	var expiration = time.Now().Unix()

	os.Unsetenv(EnvDisableCache)
	os.Setenv(EnvCacheDir, "testdata")
	os.Setenv(EnvCipherKey, cipherKey)
	defer os.Unsetenv(EnvCipherKey)

	cacheUtil := NewDefaultCacheUtil(nil)

	cacheUtil.CacheNewToken(&CachedToken{
		Token:      token,
		Expiration: time.Now().Unix(),
	}, method)

	cachedToken := decryptJSONFile(t, cacheUtil.TokenFilename(method), cipherKey)

	if cachedToken.Token != token {
		t.Fatalf("expected cached token ID of %q but got %q", token, cachedToken.Token)
	}
	if cachedToken.Expiration != expiration {
		t.Fatalf("expected cached token expiration date of %d but got %d instead",
			expiration, cachedToken.Expiration)
	}
}

func TestNullCacheUtil_GetCacheDir(t *testing.T) {
	const cacheDir = "testdata"

	os.Setenv(EnvDisableCache, "true")
	os.Setenv(EnvCacheDir, cacheDir)

	cacheUtil := NewNullCacheUtil()
	if cacheUtil.GetCacheDir() != cacheDir {
		t.Fatalf("Expected cacheUtil.cacheDir to be %q, but got %q instead",
			cacheDir, cacheUtil.cacheDir)
	}
}

// func TestNullCacheUtil_GetCachedToken(t *testing.T) {
// 	const cacheDir = "testdata"

// 	os.Setenv(EnvDisableCache, "true")
// 	os.Setenv(EnvCacheDir, cacheDir)

// 	cacheUtil := NewNullCacheUtil()
// 	token := cacheUtil.GetCachedToken(config.VaultAuthMethodAWSIAM)
// 	if token != "" {
// 		t.Fatal("expected an empty string")
// 	}
// }

func TestNullCacheUtil_LookupToken(t *testing.T) {
	const cacheDir = "testdata"

	os.Setenv(EnvDisableCache, "true")
	os.Setenv(EnvCacheDir, cacheDir)

	cacheUtil := NewNullCacheUtil()
	token, err := cacheUtil.LookupToken(config.VaultAuthMethodAWSIAM)
	if err != nil {
		t.Fatal("expected a nil error")
	}
	if token != nil {
		t.Fatal("expected a nil *CachedToken value")
	}
}

func TestNullCacheUtil_CacheNewToken(t *testing.T) {
	const cacheDir = "testdata"

	os.Setenv(EnvDisableCache, "true")
	os.Setenv(EnvCacheDir, cacheDir)

	cacheUtil := NewNullCacheUtil()

	err := cacheUtil.CacheNewToken("", config.VaultAuthMethodAWSIAM)
	if err != nil {
		t.Fatal("expected a nil error")
	}
}

func TestNullCacheUtil_RenewToken(t *testing.T) {
	const cacheDir = "testdata"

	os.Setenv(EnvDisableCache, "true")
	os.Setenv(EnvCacheDir, cacheDir)

	cacheUtil := NewNullCacheUtil()

	err := cacheUtil.RenewToken(nil)
	if err != nil {
		t.Fatal("expected a nil error")
	}
}

func TestNullCacheUtil_ClearCachedToken(t *testing.T) {
	const cacheDir = "testdata"

	os.Setenv(EnvDisableCache, "true")
	os.Setenv(EnvCacheDir, cacheDir)

	cacheUtil := NewNullCacheUtil()

	// Should return nothing and have no effect at all
	cacheUtil.ClearCachedToken(config.VaultAuthMethodAWSIAM)
}

func TestNullCacheUtil_TokenFilename(t *testing.T) {
	const cacheDir = "testdata"

	os.Setenv(EnvDisableCache, "true")
	os.Setenv(EnvCacheDir, cacheDir)

	cacheUtil := NewNullCacheUtil()

	fname := cacheUtil.TokenFilename(config.VaultAuthMethodAWSIAM)
	if fname != "" {
		t.Fatal("expected an empty string")
	}
}

func TestMain(m *testing.M) {
	logging.SetupTestLogger()
	status := m.Run()
	os.Unsetenv(EnvDisableCache)
	os.Setenv(EnvCacheDir, "testdata")
	cacheUtil := NewCacheUtil(nil)
	methods := []config.VaultAuthMethod{
		config.VaultAuthMethodAWSIAM,
		config.VaultAuthMethodAWSEC2,
	}
	for _, method := range methods {
		cacheUtil.ClearCachedToken(method)
	}
	os.Exit(status)
}

func writeJSONToFile(t *testing.T, json map[string]interface{}, tokenfile string) {
	data, err := jsonutil.EncodeJSON(json)
	if err != nil {
		t.Fatal(err)
	}
	writeDataToFile(t, data, tokenfile)
}

func encryptJSONAndWriteFile(t *testing.T, json map[string]interface{}, tokenfile, key string) {
	data, err := jsonutil.EncodeJSON(json)
	if err != nil {
		t.Fatal(err)
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		t.Fatal(err)
	}

	ciphertext := make([]byte, aes.BlockSize+len(data))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		t.Fatal(err)
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], data)

	writeDataToFile(t, ciphertext, tokenfile)
}

func decryptJSONFile(t *testing.T, tokenfile, key string) *CachedToken {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		t.Fatal(err)
	}

	ciphertext, err := ioutil.ReadFile(tokenfile)
	if err != nil {
		t.Fatal(err)
	}

	if len(ciphertext) < aes.BlockSize {
		t.Fatal("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]
	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)

	var token = new(CachedToken)
	if err = jsonutil.DecodeJSON(ciphertext, token); err != nil {
		t.Fatal(err)
	}
	return token
}

func writeDataToFile(t *testing.T, data []byte, tokenfile string) {
	if err := os.MkdirAll(filepath.Dir(tokenfile), 0755); err != nil {
		t.Fatal(err)
	}

	file, err := os.OpenFile(tokenfile, os.O_WRONLY|os.O_CREATE, 0664)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		t.Fatal(err)
	}
}

func loadTokenFromFile(t *testing.T, filename string) *CachedToken {
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var token = new(CachedToken)
	if err = jsonutil.DecodeJSONFromReader(file, token); err != nil {
		t.Fatal(err)
	}
	return token
}
