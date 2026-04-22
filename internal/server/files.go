package server

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/dmiller/foyer/internal/auth"
	"github.com/dmiller/foyer/internal/id"
)

const defaultMaxUploadBytes = 64 << 30 // 64 GB

func uploadFileHandler(db *sql.DB, dataDir string) http.HandlerFunc {
	maxUpload := defaultMaxUploadBytes

	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)

		r.Body = http.MaxBytesReader(w, r.Body, int64(maxUpload)+1024) // extra for form overhead
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "file too large or invalid form", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Parse expiry (hours)
		expiryHours, _ := strconv.Atoi(r.FormValue("expiry_hours"))
		if expiryHours <= 0 {
			expiryHours = 24 // default 1 day
		}
		if expiryHours > 168 { // max 7 days
			expiryHours = 168
		}
		expiresAt := time.Now().Add(time.Duration(expiryHours) * time.Hour)

		// Parse max downloads
		var maxDownloads sql.NullInt64
		if md := r.FormValue("max_downloads"); md != "" {
			val, err := strconv.ParseInt(md, 10, 64)
			if err == nil && val > 0 {
				maxDownloads = sql.NullInt64{Int64: val, Valid: true}
			}
		}

		// Optional password
		var passwordHash sql.NullString
		if pw := r.FormValue("password"); pw != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(pw), 10)
			if err != nil {
				slog.Error("failed to hash file password", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			passwordHash = sql.NullString{String: string(hash), Valid: true}
		}

		// Detect MIME type
		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" || mimeType == "application/octet-stream" {
			mimeType = mime.TypeByExtension(filepath.Ext(header.Filename))
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
		}

		// Generate ID and save file
		fileID := id.New()
		storageName := fileID // flat storage, no subdirs
		storagePath := filepath.Join(dataDir, "files", storageName)

		dst, err := os.Create(storagePath)
		if err != nil {
			slog.Error("failed to create file", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		written, err := io.Copy(dst, file)
		dst.Close()
		if err != nil {
			os.Remove(storagePath)
			slog.Error("failed to write file", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Insert into database
		_, err = db.Exec(
			`INSERT INTO files (id, filename, size_bytes, mime_type, storage_path, password_hash, max_downloads, uploaded_by, expires_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fileID, header.Filename, written, mimeType, storageName,
			passwordHash, maxDownloads, username, expiresAt.UTC().Format(time.RFC3339),
		)
		if err != nil {
			os.Remove(storagePath)
			slog.Error("failed to insert file record", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, map[string]interface{}{
			"id":         fileID,
			"filename":   header.Filename,
			"size_bytes": written,
			"url":        fmt.Sprintf("/d/%s", fileID),
			"expires_at": expiresAt.UTC().Format(time.RFC3339),
		})
	}
}

func listFilesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)
		rows, err := db.Query(
			`SELECT id, filename, size_bytes, mime_type, download_count, max_downloads, expires_at, created_at
			 FROM files WHERE uploaded_by = ? ORDER BY created_at DESC`,
			username,
		)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		files, err := scanRows(rows, func(rows *sql.Rows) (map[string]interface{}, error) {
			var id, filename, mimeType, expiresAt, createdAt string
			var sizeBytes int64
			var downloadCount int
			var maxDownloads sql.NullInt64
			err := rows.Scan(&id, &filename, &sizeBytes, &mimeType, &downloadCount, &maxDownloads, &expiresAt, &createdAt)
			m := map[string]interface{}{
				"id": id, "filename": filename, "size_bytes": sizeBytes, "mime_type": mimeType,
				"download_count": downloadCount, "expires_at": expiresAt, "created_at": createdAt,
				"url": fmt.Sprintf("/d/%s", id),
			}
			if maxDownloads.Valid {
				m["max_downloads"] = maxDownloads.Int64
			}
			return m, err
		})
		if err != nil {
			slog.Error("row iteration error", "error", err)
		}
		writeJSON(w, files)
	}
}

func deleteFileHandler(db *sql.DB, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)
		fileID := chi.URLParam(r, "id")

		var storagePath, uploadedBy string
		err := db.QueryRow("SELECT storage_path, uploaded_by FROM files WHERE id = ?", fileID).Scan(&storagePath, &uploadedBy)
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Only the uploader or admin can delete
		if uploadedBy != username && !auth.IsAdmin(claims) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		os.Remove(filepath.Join(dataDir, "files", storagePath))
		db.Exec("DELETE FROM files WHERE id = ?", fileID)
		w.WriteHeader(http.StatusNoContent)
	}
}

func downloadFileHandler(db *sql.DB, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileID := chi.URLParam(r, "id")

		var filename, mimeType, storagePath string
		var passwordHash sql.NullString
		var maxDownloads sql.NullInt64
		var downloadCount int
		var expiresAt string

		err := db.QueryRow(
			`SELECT filename, mime_type, storage_path, password_hash, max_downloads, download_count, expires_at
			 FROM files WHERE id = ?`, fileID,
		).Scan(&filename, &mimeType, &storagePath, &passwordHash, &maxDownloads, &downloadCount, &expiresAt)

		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Check expiry
		expiry, _ := time.Parse(time.RFC3339, expiresAt)
		if time.Now().After(expiry) {
			http.Error(w, "file has expired", http.StatusGone)
			return
		}

		// Check download limit
		if maxDownloads.Valid && int64(downloadCount) >= maxDownloads.Int64 {
			http.Error(w, "download limit reached", http.StatusGone)
			return
		}

		// Check password if set
		if passwordHash.Valid {
			password := r.URL.Query().Get("password")
			if password == "" {
				password = r.Header.Get("X-File-Password")
			}
			if password == "" || bcrypt.CompareHashAndPassword([]byte(passwordHash.String), []byte(password)) != nil {
				http.Error(w, "password required", http.StatusUnauthorized)
				return
			}
		}

		// Increment download count
		db.Exec("UPDATE files SET download_count = download_count + 1 WHERE id = ?", fileID)

		fullPath := filepath.Join(dataDir, "files", storagePath)
		f, err := os.Open(fullPath)
		if err != nil {
			http.Error(w, "file not found on disk", http.StatusNotFound)
			return
		}
		defer f.Close()

		// Sanitize filename for Content-Disposition
		safeName := strings.ReplaceAll(filename, "\"", "'")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
		w.Header().Set("Content-Type", mimeType)
		io.Copy(w, f)
	}
}

func fileInfoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileID := chi.URLParam(r, "id")

		var filename, mimeType, expiresAt string
		var sizeBytes int64
		var passwordHash sql.NullString
		var maxDownloads sql.NullInt64
		var downloadCount int

		err := db.QueryRow(
			`SELECT filename, size_bytes, mime_type, password_hash, max_downloads, download_count, expires_at
			 FROM files WHERE id = ?`, fileID,
		).Scan(&filename, &sizeBytes, &mimeType, &passwordHash, &maxDownloads, &downloadCount, &expiresAt)

		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		expiry, _ := time.Parse(time.RFC3339, expiresAt)
		expired := time.Now().After(expiry)
		limitReached := maxDownloads.Valid && int64(downloadCount) >= maxDownloads.Int64

		info := map[string]interface{}{
			"filename":          filename,
			"size_bytes":        sizeBytes,
			"mime_type":         mimeType,
			"requires_password": passwordHash.Valid,
			"expired":           expired,
			"limit_reached":     limitReached,
			"download_count":    downloadCount,
			"expires_at":        expiresAt,
		}
		if maxDownloads.Valid {
			info["max_downloads"] = maxDownloads.Int64
		}
		writeJSON(w, info)
	}
}
