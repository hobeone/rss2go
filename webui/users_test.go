package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

const testGetUserGoldenResponse = `{
  "user": {
    "id": 4,
    "name": "test",
    "email": "test@test.com",
    "enabled": true,
    "feeds": []
  }
}`

func TestGetUser(t *testing.T) {
	dbh, m := setupTest(t)
	response := httptest.NewRecorder()

	user, err := dbh.AddUser("test", "test@test.com", "pass")
	if err != nil {
		t.Fatalf("Couldn't create user: %s", err)
	}

	req, err := http.NewRequest("GET",
		fmt.Sprintf("/api/1/users/%d", user.ID), nil)
	if err != nil {
		t.Fatalf("Error creating request: %s", err)
	}

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}

	if response.Body.String() != testGetUserGoldenResponse {
		fmt.Println(response.Body.String())
		t.Fatalf("Response doesn't match golden response")
	}

	// Unknwon User
	req, _ = http.NewRequest("GET", "/api/1/users/-1", nil)
	response = httptest.NewRecorder()
	m.ServeHTTP(response, req)

	if response.Code != 404 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 404 response code, got %d", response.Code)
	}
}

const testAllUsersGoldenResponse = `{
  "users": [
    {
      "id": 1,
      "name": "testuser1",
      "email": "test1@example.com",
      "enabled": true,
      "feeds": [
        1,
        2,
        3
      ]
    },
    {
      "id": 2,
      "name": "testuser2",
      "email": "test2@example.com",
      "enabled": true,
      "feeds": [
        1,
        2,
        3
      ]
    },
    {
      "id": 3,
      "name": "testuser3",
      "email": "test3@example.com",
      "enabled": true,
      "feeds": [
        1,
        2,
        3
      ]
    }
  ]
}`

func TestGetAllUsers(t *testing.T) {
	_, m := setupTest(t)
	RegisterTestingT(t)

	response := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/1/users", nil)
	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}

	Expect(response.Body.String()).Should(MatchJSON(testAllUsersGoldenResponse))
}

const getSomeUsersGoldenResponse = `{
  "users": [
    {
      "id": 1,
      "name": "testuser1",
      "email": "test1@example.com",
      "enabled": true,
      "feeds": [
        1,
        2,
        3
      ]
    },
    {
      "id": 2,
      "name": "testuser2",
      "email": "test2@example.com",
      "enabled": true,
      "feeds": [
        1,
        2,
        3
      ]
    }
  ]
}`

func TestGetSomeUsers(t *testing.T) {
	_, m := setupTest(t)
	RegisterTestingT(t)

	response := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/1/users?ids[]=1&ids[]=2", nil)
	failOnError(t, err)
	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}

	Expect(response.Body.String()).Should(MatchJSON(getSomeUsersGoldenResponse))
}

const updateUserReq = `
{"user":{"name":"test1","email":"test1_changed@example.com","feeds":[1,2,3]}}
`

func TestUpdateUser(t *testing.T) {
	dbh, m := setupTest(t)

	_, err := dbh.GetUserByEmail("test1@example.com")
	failOnError(t, err)

	response := httptest.NewRecorder()
	body := strings.NewReader(updateUserReq)
	req, err := http.NewRequest("PUT", "/api/1/users/1", body)
	failOnError(t, err)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}

	_, err = dbh.GetUserByEmail("test1_changed@example.com")
	failOnError(t, err)
}

const addUserGoldenOutput = `{
  "user": {
    "id": 4,
    "name": "test1",
    "email": "test1_changed@example.com",
    "enabled": true,
    "feeds": []
  }
}`

func TestAddUser(t *testing.T) {
	RegisterTestingT(t)
	u := unmarshalUserJSONContainer{
		unmarshalUserJSON{
			ID:       4,
			Name:     "test1",
			Email:    "test1_changed@example.com",
			Password: "123",
			Feeds:    []int64{},
		},
	}
	encoded, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Error marshaling json: %s", err)
	}

	dbh, m := setupTest(t)
	response := httptest.NewRecorder()
	body := strings.NewReader(string(encoded[:]))
	req, _ := http.NewRequest("POST", "/api/1/users", body)
	req.Header["Content-Type"] = []string{"application/json; charset=UTF-8"}

	m.ServeHTTP(response, req)

	if response.Code != 201 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 201 response code, got %d", response.Code)
	}

	dbh.GetUserByEmail("test1_changed@example.com")

	Expect(response.Body.String()).Should(MatchJSON(addUserGoldenOutput))

	if response.Header().Get("Location") != "/users/4" {
		t.Fatalf("Expected location of '/users/4' got %s",
			response.Header().Get("Location"))
	}
}

func TestDeleteUser(t *testing.T) {
	dbh, m := setupTest(t)

	users, err := dbh.GetAllUsers()
	failOnError(t, err)

	req, _ := http.NewRequest("DELETE",
		fmt.Sprintf("/api/1/users/%d", users[0].ID), nil)
	response := httptest.NewRecorder()
	m.ServeHTTP(response, req)
	if response.Code != 204 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 204 response code, got %d", response.Code)
	}
	_, err = dbh.GetUserByID(users[0].ID)
	if err == nil {
		t.Fatalf("Found user when it should have been deleted")
	}
}
