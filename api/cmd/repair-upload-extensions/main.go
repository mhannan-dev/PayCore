// Repairs extensionless attachment keys without exposing uploads publicly.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/database/migrations"
	"paycore/api/internal/config"
	"paycore/api/internal/database"
	"paycore/api/internal/storage"
)

type legacyFile struct {
	id, key, name, digest string
	size                  int64
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = migrations.Apply(ctx, pool); err != nil {
		return err
	}
	root, err := os.OpenRoot(cfg.UploadDir)
	if err != nil {
		return err
	}
	defer root.Close()
	rows, err := pool.Query(ctx, `SELECT id, storage_key, file_name, sha256, size_bytes
        FROM attachments WHERE storage_kind='LOCAL' AND length(storage_key)=64`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var files []legacyFile
	for rows.Next() {
		var file legacyFile
		if err = rows.Scan(&file.id, &file.key, &file.name, &file.digest, &file.size); err != nil {
			return err
		}
		files = append(files, file)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	for _, file := range files {
		if err = repair(ctx, pool, root, file); err != nil {
			return fmt.Errorf("repair attachment %s: %w", file.id, err)
		}
	}
	log.Printf("Upload extensions repaired: %d. Unreferenced files were left untouched.", len(files))
	return nil
}

func repair(ctx context.Context, pool *pgxpool.Pool, root *os.Root, file legacyFile) error {
	decoded, err := hex.DecodeString(file.key)
	if err != nil || len(decoded) != 32 {
		return errors.New("invalid legacy key")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var current string
	if err = tx.QueryRow(ctx, "SELECT storage_key FROM attachments WHERE id=$1 FOR UPDATE", file.id).Scan(&current); err != nil {
		return err
	}
	if current != file.key {
		return nil
	}
	newKey := file.key + storage.FileExtension(file.name)
	if err = copyVerified(root, file, newKey); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE attachments SET storage_key=$2 WHERE id=$1", file.id, newKey); err != nil {
		return err
	}
	// Keep the original on any uncertain commit so an existing DB reference works.
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if err = root.Remove(file.key); err != nil {
		log.Printf("Attachment %s repaired; original copy retained because cleanup failed.", file.id)
	}
	return nil
}

func copyVerified(root *os.Root, file legacyFile, newKey string) error {
	source, err := root.Open(file.key)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := root.OpenFile(newKey, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		existing, openErr := root.Open(newKey)
		if openErr != nil {
			return openErr
		}
		defer existing.Close()
		return verify(existing, io.Discard, file)
	}
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		target.Close()
		if !complete {
			_ = root.Remove(newKey)
		}
	}()
	if err = verify(source, target, file); err != nil {
		return err
	}
	if err = target.Sync(); err != nil {
		return err
	}
	if err = target.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func verify(source io.Reader, target io.Writer, file legacyFile) error {
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(target, hash), io.LimitReader(source, storage.MaxFileSize+1))
	if err != nil {
		return err
	}
	if size != file.size || hex.EncodeToString(hash.Sum(nil)) != file.digest {
		return errors.New("stored content does not match attachment checksum and size")
	}
	return nil
}
