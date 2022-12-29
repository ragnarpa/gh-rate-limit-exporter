package exporter

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func Cwd(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.FailNow()
	}

	return cwd
}

func NewTestFS(t *testing.T) *afero.Afero {
	fs := &afero.Afero{Fs: afero.NewMemMapFs()}
	err := fs.MkdirAll(Cwd(t), 0700)
	if err != nil {
		t.FailNow()
	}

	return fs
}

//go:embed testdata/test-credentials.yml
var credentials string

func writeCredentials(b []byte, t *testing.T, fs *afero.Afero) {
	fs.WriteFile(filepath.Join(Cwd(t), FileCredentialFileName), b, 0600)
}

func TestFileCredentialSource(t *testing.T) {
	t.Parallel()

	t.Run("reads credentials from credential file", func(t *testing.T) {
		fs := NewTestFS(t)
		writeCredentials([]byte(credentials), t, fs)

		var src CredentialSource
		src, err := NewFileCredentialSource(fs)
		credentials := src.Credentials()

		assert.NoError(t, err)
		assert.Len(t, credentials, 2)
	})

	t.Run("returns error with malformed credential file", func(t *testing.T) {
		fs := NewTestFS(t)
		writeCredentials([]byte("..."), t, fs)

		src, err := NewFileCredentialSource(fs)

		assert.Nil(t, src)
		assert.EqualError(t, err, "yaml: did not find expected node content")
	})

	t.Run("returns error if credential file does not exist", func(t *testing.T) {
		fs := NewTestFS(t)

		src, err := NewFileCredentialSource(fs)

		assert.Nil(t, src)
		if assert.Error(t, err) {
			target := &os.PathError{}
			assert.ErrorAs(t, err, &target)
			assert.Equal(t, filepath.Join(Cwd(t), FileCredentialFileName), target.Path)
		}
	})
}

func TestCredential(t *testing.T) {
	find := func(name string, credentials []*Credential) *Credential {
		for _, c := range credentials {
			if c.AppName == name {
				return c
			}
		}

		return nil
	}

	assertIsMyAppOne := func(t *testing.T, c *Credential) {
		assert.NotNil(t, c)
		assert.Equal(t, "gh-app", c.Kind())
		assert.Equal(t, "my-app-one", c.Name())
		assert.Equal(t, int64(1), c.ID())
		assert.Equal(t, int64(2), c.InstallationID())
		assert.Equal(t, "key", c.Base64PrivateKey())
		assert.Nil(t, c.PAT)
	}

	assertIsMyAppTwo := func(t *testing.T, c *Credential) {
		assert.NotNil(t, c)
		assert.Equal(t, "gh-pat", c.Kind())
		assert.Equal(t, "my-app-two", c.Name())
		assert.Equal(t, "token", c.Token())
		assert.Nil(t, c.AppCredential)
	}

	t.Run("credentials are decoded correctly", func(t *testing.T) {
		fs := NewTestFS(t)
		writeCredentials([]byte(credentials), t, fs)

		var src CredentialSource
		src, err := NewFileCredentialSource(fs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		credentials := src.Credentials()

		assertIsMyAppOne(t, find("my-app-one", credentials))
		assertIsMyAppTwo(t, find("my-app-two", credentials))
	})
}
