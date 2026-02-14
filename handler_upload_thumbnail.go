package main

import (
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

const (
	MiB       = 1 << 20
	maxMemory = 10 * MiB
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoID, userID, err := validateRequest(cfg, w, r)
	if err != nil {
		log.Println("Error: could not validate request:", err)
		return
	}

	log.Println("uploading thumbnail for video", videoID, "by user", userID)

	mediaType, multipartFile, err := parseThumbnailReq(w, r)
	if err != nil {
		log.Println("Error: could not parse request:", err)
		return
	}

	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find image metdata in db", err)
		return
	} else if metadata.UserID != userID {
		http.Error(w, "User ID from request does not match video owner ID", http.StatusUnauthorized)
		return
	}
	log.Println("Info: image metdata retrieved from db and user ID verified")

	if err := cfg.updateThumbnail(videoID, multipartFile, mediaType, metadata); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update thumbnail", err)
		log.Println("Error: could not update thumbnail:", err)
		return
	}

	log.Println("Info: thumbnail successfully set")
	respondWithJSON(w, http.StatusOK, metadata)
}

func validateRequest(cfg *apiConfig, w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, error) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return uuid.UUID{}, uuid.UUID{}, err
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return uuid.UUID{}, uuid.UUID{}, err
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return uuid.UUID{}, uuid.UUID{}, err
	}

	return videoID, userID, nil
}

func parseThumbnailReq(w http.ResponseWriter, r *http.Request) (string, multipart.File, error) {
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse multipart form", err)
		return "", nil, err
	}

	multipartFile, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't form file the request", err)
		return "", nil, err
	}

	contentType := strings.Split(header.Header.Get("Content-Type"), "/")
	mediaType := contentType[1]

	return mediaType, multipartFile, nil
}

func (cfg *apiConfig) updateThumbnail(videoID uuid.UUID, multipartFile multipart.File, mediaType string, metadata database.Video) error {
	fileName := videoID.String() + "." + mediaType
	thumbnailFilePath := filepath.Join(cfg.assetsRoot, fileName)

	file, err := os.Create(thumbnailFilePath)
	if err != nil {
		return err
	}

	if _, err = io.Copy(file, multipartFile); err != nil {
		return err
	}

	tnURL := "http://localhost:" + cfg.port + "/assets/" + fileName
	metadata.ThumbnailURL = &tnURL
	cfg.db.UpdateVideo(metadata)

	log.Println("Info: thumbnail URL updated in db")
	return nil
}
