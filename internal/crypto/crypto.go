package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

type KeyPair struct {
	PrivateKey []byte `json:"private_key"`
	PublicKey  []byte `json:"public_key"`
}

type KeyStore struct {
	Keys KeyPair `json:"keys"`
}

func GenerateKeyPair() (*KeyPair, error) {
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return nil, err
	}
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	publicKey, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return nil, err
	}

	return &KeyPair{PrivateKey: privateKey[:], PublicKey: publicKey}, nil
}

func LoadOrCreateKeys(path string) (*KeyPair, error) {
	if data, err := os.ReadFile(path); err == nil {
		var ks KeyStore
		if err := json.Unmarshal(data, &ks); err == nil && len(ks.Keys.PrivateKey) == 32 {
			return &ks.Keys, nil
		}
	}

	kp, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	ks := KeyStore{Keys: *kp}
	data, err := json.Marshal(ks)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return nil, err
	}
	return kp, nil
}

func SharedSecret(privateKey, publicKey []byte) ([]byte, error) {
	return curve25519.X25519(privateKey, publicKey)
}

func DeriveKey(sharedSecret []byte) []byte {
	salt := []byte("techat-v1")
	info := []byte("encryption-key")
	hkdf := hkdf.New(sha256.New, sharedSecret, salt, info)
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf, key); err != nil {
		hash := sha256.Sum256(sharedSecret)
		return hash[:]
	}
	return key
}

func Encrypt(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = aesGCM.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func Decrypt(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func EncryptMessage(key []byte, plaintext string) (cipherText, nonceStr string, err error) {
	ct, n, err := Encrypt(key, []byte(plaintext))
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(ct), base64.StdEncoding.EncodeToString(n), nil
}

func DecryptMessage(key []byte, cipherText, nonceStr string) (string, error) {
	ct, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}
	n, err := base64.StdEncoding.DecodeString(nonceStr)
	if err != nil {
		return "", err
	}
	pt, err := Decrypt(key, ct, n)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func WipeKeyFile(path string) error {
	for i := 0; i < 3; i++ {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		zeros := make([]byte, info.Size())
		if _, err := f.Write(zeros); err != nil {
			f.Close()
			return err
		}
		f.Sync()
		f.Close()
	}
	return os.Remove(path)
}
