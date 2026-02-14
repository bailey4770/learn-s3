package main

import (
	"encoding/base64"
	"io"
	"log"
	"net/http"

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

	mediaType, imageData, err := parseThumbnailReq(w, r)
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

	cfg.updateThumbnail(videoID, imageData, mediaType, metadata)
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

func parseThumbnailReq(w http.ResponseWriter, r *http.Request) (string, []byte, error) {
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse multipart form", err)
		return "", nil, err
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't form file the request", err)
		return "", nil, err
	}

	mediaType := header.Header.Get("Content-Type")

	imageData, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read image data", err)
		return "", nil, err
	}

	return mediaType, imageData, nil
}

func (cfg *apiConfig) updateThumbnail(videoID uuid.UUID, imageData []byte, mediaType string, metadata database.Video) {
	encoded := base64.StdEncoding.EncodeToString(imageData)
	tnURL := "data:" + mediaType + ";base64," + encoded

	metadata.ThumbnailURL = &tnURL
	cfg.db.UpdateVideo(metadata)

	log.Println("Info: thumbnail URL updated in db")
}
