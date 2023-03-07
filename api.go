package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
)

type APIServer struct {
	listenAddress string
	storage       Storage
}

func NewAPIServer(listenAddr string, store Storage) *APIServer {
	return &APIServer{
		listenAddress: listenAddr,
		storage:       store,
	}
}

func (s *APIServer) Run() {
	router := mux.NewRouter()

	router.HandleFunc("/account", makeHTTPHandleFunc(s.HandleAccount))
	router.HandleFunc("/account/{id}", makeHTTPHandleFunc(withJWTAuth(s.HandleGetAccountByID, s.storage)))
	router.HandleFunc("/transfer", makeHTTPHandleFunc(withJWTAuth(s.HandleTransfer, s.storage)))

	log.Println("JSON API Server is running on ", s.listenAddress)

	http.ListenAndServe(s.listenAddress, router)
}

func (s *APIServer) HandleAccount(w http.ResponseWriter, r *http.Request) error {
	if r.Method == http.MethodGet {
		return s.HandleGetAccount(w, r)
	}
	if r.Method == http.MethodPost {
		return s.HandleCreateAccount(w, r)
	}

	return fmt.Errorf("method not allowed %s", r.Method)
}

func (s *APIServer) HandleGetAccount(w http.ResponseWriter, r *http.Request) error {
	accounts, err := s.storage.GetAccounts()
	if err != nil {
		return err
	}

	return writeJSON(w, http.StatusOK, accounts)
}

func (s *APIServer) HandleGetAccountByID(w http.ResponseWriter, r *http.Request) error {
	if r.Method == http.MethodGet {
		id, err := getID(r)
		if err != nil {
			return err
		}

		account, err := s.storage.GetAccountByID(id)
		if err != nil {
			return err
		}

		return writeJSON(w, http.StatusOK, account)
	}

	if r.Method == http.MethodDelete {
		return s.HandleDeleteAccount(w, r)
	}

	return fmt.Errorf("method not allowed")
}

func (s *APIServer) HandleCreateAccount(w http.ResponseWriter, r *http.Request) error {
	createAccountRequest := new(CreateAccountRequest)
	if err := json.NewDecoder(r.Body).Decode(createAccountRequest); err != nil {
		return err
	}
	defer r.Body.Close()

	account := NewAccount(createAccountRequest.FirstName, createAccountRequest.LastName)

	if err := s.storage.CreateAccount(account); err != nil {
		return err
	}

	tokenString, err := createJWT(account)
	if err != nil {
		return err
	}

	fmt.Println("Token: ", tokenString)

	return writeJSON(w, http.StatusOK, account)
}

func (s *APIServer) HandleDeleteAccount(w http.ResponseWriter, r *http.Request) error {
	id, err := getID(r)
	if err != nil {
		return err
	}

	if err := s.storage.DeleteAccount(id); err != nil {
		return err
	}

	return writeJSON(w, http.StatusOK, map[string]int{"deleted": id})
}

func (s *APIServer) HandleTransfer(w http.ResponseWriter, r *http.Request) error {
	transferReq := new(TransferRequest)
	if err := json.NewDecoder(r.Body).Decode(transferReq); err != nil {
		return err
	}
	defer r.Body.Close()

	return writeJSON(w, http.StatusOK, transferReq)
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)

	return json.NewEncoder(w).Encode(v)
}

func createJWT(account *Account) (string, error) {
	claims := jwt.MapClaims{
		"expiresAt":     15000,
		"accountNumber": account.Number,
	}

	secret := getSecret()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

// eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhY2NvdW50TlVtYmVyIjo1Njg5NDc3ODIsImV4cGlyZXNBdCI6MTUwMDB9.cxWkzShHPDvyEqHNCUzCvILFg3kyq80DNdfOO8YpW_I
func withJWTAuth(apiFunc apiFunc, s Storage) apiFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		tokenString := r.Header.Get("x-jwt-token")
		token, err := validateJWT(tokenString)
		if err != nil || !token.Valid {
			return permissionDenied
		}

		userId, err := getID(r)
		if err != nil {
			return permissionDenied
		}

		account, err := s.GetAccountByID(userId)
		if err != nil {
			return permissionDenied
		}

		claims := token.Claims.(jwt.MapClaims)
		res, ok := claims["accountNumber"].(float64)
		if !ok || account.Number != int32(res) {
			return permissionDenied
		}

		err = apiFunc(w, r)
		if err != nil {
			return err
		}

		return nil
	}
}

func validateJWT(tokenString string) (*jwt.Token, error) {
	secret := getSecret()

	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(secret), nil
	})
}

var permissionDenied = ApiError{Err: "permission denied", Status: http.StatusForbidden}

type apiFunc func(http.ResponseWriter, *http.Request) error

type ApiError struct {
	Err    string `json:"error"`
	Status int    `json:"code"`
}

func (e ApiError) Error() string {
	return e.Err
}

func makeHTTPHandleFunc(f apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		er := f(w, r)
		if er != nil {
			e, ok := er.(ApiError)
			if ok {
				writeJSON(w, e.Status, e)
				return
			}
			fmt.Println("Internal error: ", er.Error())
			writeJSON(w, http.StatusInternalServerError, ApiError{Err: "internal server", Status: http.StatusInternalServerError})
		}
	}
}

func getSecret() string {
	return os.Getenv("JWT_SECRET")
}

func getID(r *http.Request) (int, error) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return id, fmt.Errorf("invalid id given: %s", idStr)
	}
	return id, err
}
