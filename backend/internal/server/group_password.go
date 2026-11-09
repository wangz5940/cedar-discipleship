package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
)

func (a *app) defaultPasswordCipher() (cipher.AEAD, error) {
	key := sha256.Sum256(append([]byte("cedar/group-default-password/v1/"), a.secret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (a *app) encryptDefaultPassword(password string) (string, error) {
	box, err := a.defaultPasswordCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, box.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(box.Seal(nonce, nonce, []byte(password), nil)), nil
}

func (a *app) decryptDefaultPassword(encrypted string) (string, error) {
	box, err := a.defaultPasswordCipher()
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil || len(data) < box.NonceSize()+box.Overhead() {
		return "", errors.New("invalid_encrypted_password")
	}
	plain, err := box.Open(nil, data[:box.NonceSize()], data[box.NonceSize():], nil)
	return string(plain), err
}

func (a *app) handleAdminGetGroupDefaultPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	groupID := requireGroupID(w, mustUser(r))
	if groupID == 0 {
		return
	}
	var encrypted sql.NullString
	if err := a.db.QueryRowContext(r.Context(), `SELECT default_password_encrypted FROM study_groups WHERE id=?`, groupID).Scan(&encrypted); err != nil {
		writeError(w, http.StatusInternalServerError, "default_password_read_failed")
		return
	}
	if !encrypted.Valid || encrypted.String == "" {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	password, err := a.decryptDefaultPassword(encrypted.String)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "默认密码无法解密，请重新设置默认密码")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "password": password})
}
