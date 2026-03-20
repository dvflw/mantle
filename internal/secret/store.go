package secret

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/auth"
)

// Store handles CRUD operations for encrypted credentials in Postgres.
type Store struct {
	DB        *sql.DB
	Encryptor *Encryptor
}

// Create validates, encrypts, and stores a new credential.
func (s *Store) Create(ctx context.Context, name, typeName string, data map[string]string) (*Credential, error) {
	ct, err := GetType(typeName)
	if err != nil {
		return nil, err
	}
	if err := ct.Validate(data); err != nil {
		return nil, err
	}

	plaintext, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshaling credential data: %w", err)
	}

	ciphertext, nonce, err := s.Encryptor.Encrypt(plaintext)
	if err != nil {
		return nil, fmt.Errorf("encrypting credential: %w", err)
	}

	teamID := auth.TeamIDFromContext(ctx)

	var cred Credential
	err = s.DB.QueryRowContext(ctx,
		`INSERT INTO credentials (name, type, encrypted_data, nonce, team_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, name, type, created_at, updated_at`,
		name, typeName, ciphertext, nonce, teamID,
	).Scan(&cred.ID, &cred.Name, &cred.Type, &cred.CreatedAt, &cred.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("storing credential: %w", err)
	}

	return &cred, nil
}

// Get retrieves and decrypts a credential's field data by name.
func (s *Store) Get(ctx context.Context, name string) (map[string]string, error) {
	teamID := auth.TeamIDFromContext(ctx)
	var encryptedData, nonce []byte
	err := s.DB.QueryRowContext(ctx,
		`SELECT encrypted_data, nonce FROM credentials WHERE name = $1 AND team_id = $2`, name, teamID,
	).Scan(&encryptedData, &nonce)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("credential %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("querying credential: %w", err)
	}

	plaintext, err := s.Encryptor.Decrypt(encryptedData, nonce)
	if err != nil {
		return nil, fmt.Errorf("decrypting credential %q: %w", name, err)
	}

	var data map[string]string
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("unmarshaling credential data: %w", err)
	}

	return data, nil
}

// List returns all credentials (metadata only — never decrypted values).
func (s *Store) List(ctx context.Context) ([]Credential, error) {
	teamID := auth.TeamIDFromContext(ctx)
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, name, type, created_at, updated_at FROM credentials WHERE team_id = $1 ORDER BY name`,
		teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing credentials: %w", err)
	}
	defer rows.Close()

	var creds []Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning credential: %w", err)
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// Delete removes a credential by name.
func (s *Store) Delete(ctx context.Context, name string) error {
	teamID := auth.TeamIDFromContext(ctx)
	result, err := s.DB.ExecContext(ctx,
		`DELETE FROM credentials WHERE name = $1 AND team_id = $2`, name, teamID,
	)
	if err != nil {
		return fmt.Errorf("deleting credential: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("credential %q not found", name)
	}
	return nil
}

// ReEncryptAll decrypts all credentials with the current encryptor and
// re-encrypts them with a new encryptor. Used for key rotation.
func (s *Store) ReEncryptAll(ctx context.Context, newEncryptor *Encryptor) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	teamID := auth.TeamIDFromContext(ctx)
	rows, err := tx.QueryContext(ctx,
		`SELECT id, encrypted_data, nonce FROM credentials WHERE team_id = $1 FOR UPDATE`,
		teamID,
	)
	if err != nil {
		return 0, fmt.Errorf("querying credentials: %w", err)
	}

	type row struct {
		id   string
		data []byte
		nonce []byte
	}
	var toUpdate []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.data, &r.nonce); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning: %w", err)
		}
		toUpdate = append(toUpdate, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, r := range toUpdate {
		// Decrypt with old key.
		plaintext, err := s.Encryptor.Decrypt(r.data, r.nonce)
		if err != nil {
			return 0, fmt.Errorf("decrypting credential %s: %w", r.id, err)
		}

		// Re-encrypt with new key.
		newData, newNonce, err := newEncryptor.Encrypt(plaintext)
		if err != nil {
			return 0, fmt.Errorf("re-encrypting credential %s: %w", r.id, err)
		}

		_, err = tx.ExecContext(ctx,
			`UPDATE credentials SET encrypted_data = $1, nonce = $2, updated_at = $3 WHERE id = $4`,
			newData, newNonce, time.Now(), r.id,
		)
		if err != nil {
			return 0, fmt.Errorf("updating credential %s: %w", r.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing: %w", err)
	}

	return len(toUpdate), nil
}
