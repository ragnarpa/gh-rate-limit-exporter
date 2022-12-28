package exporter

import (
	"encoding/json"
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

func NewTestFS(t *testing.T) afero.Fs {
	fs := afero.NewMemMapFs()
	err := fs.MkdirAll(Cwd(t), 0700)
	if err != nil {
		t.FailNow()
	}

	return fs
}

func writeCredentialsInValidFormat(t *testing.T, fs afero.Fs) {
	data := `
		{
			"my-app-one": {
				"type": "gh-app",
				"appId": 1,
				"installationId": 2,
				"key": "key"
			},
			"my-app-two": {
				"type": "gh-pat",
				"token": "token"
			}
		}`

	afero.WriteFile(fs, filepath.Join(Cwd(t), FileCredentialFileName), []byte(data), 0600)
}

func writeCredentialsInMalformedFormat(t *testing.T, fs afero.Fs) {
	afero.WriteFile(fs, filepath.Join(Cwd(t), FileCredentialFileName), []byte("[]"), 0600)
}

func TestFileCredentialSource(t *testing.T) {
	t.Parallel()

	t.Run("reads credentials from credential file", func(t *testing.T) {
		fs := NewTestFS(t)
		writeCredentialsInValidFormat(t, fs)

		var src CredentialSource
		src, err := NewFileCredentialSource(fs)
		credentials := src.Credentials()

		assert.NoError(t, err)
		assert.Len(t, credentials, 2)
	})

	t.Run("returns error with malformed credential file", func(t *testing.T) {
		fs := NewTestFS(t)
		writeCredentialsInMalformedFormat(t, fs)

		src, err := NewFileCredentialSource(fs)

		assert.Nil(t, src)
		if assert.Error(t, err) {
			target := &json.UnmarshalTypeError{}
			assert.ErrorAs(t, err, &target)
		}
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
		writeCredentialsInValidFormat(t, fs)

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
