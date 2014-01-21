package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testGetUserGoldenResponse = `{
  "user": {
    "id": 1,
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
		fmt.Sprintf("/api/1/users/%d", user.Id), nil)
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
      "name": "test1",
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
      "name": "test2",
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
      "name": "test3",
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
	dbh, m := setupTest(t)
	loadFixtures(dbh)

	response := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/1/users", nil)
	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}

	if response.Body.String() != testAllUsersGoldenResponse {
		fmt.Println(response.Body.String())
		t.Fatalf("Response doesn't match golden response")
	}
}

const getSomeUsersGoldenResponse = `{
  "users": [
    {
      "id": 1,
      "name": "test1",
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
      "name": "test2",
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
	dbh, m := setupTest(t)
	loadFixtures(dbh)

	response := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/1/users?ids[]=1&ids[]=2", nil)
	failOnError(t, err)
	m.ServeHTTP(response, req)

	if response.Code != 200 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 200 response code, got %d", response.Code)
	}

	if response.Body.String() != getSomeUsersGoldenResponse {
		fmt.Println(response.Body.String())
		t.Fatalf("Response doesn't match golden response")
	}
}

const updateUserReq = `
{"user":{"name":"test1","email":"test1_changed@example.com","feeds":[154,154,154,154,154]}}
`

func TestUpdateUser(t *testing.T) {
	dbh, m := setupTest(t)
	loadFixtures(dbh)

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
    "id": 1,
    "name": "test1",
    "email": "test1_changed@example.com",
    "enabled": true,
    "feeds": []
  }
}`

func TestAddUser(t *testing.T) {
	u := unmarshalUserJSONContainer{
		unmarshalUserJSON{
			Id:       1,
			Name:     "test1",
			Email:    "test1_changed@example.com",
			Password: "123",
			Feeds:    []int{},
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

	if response.Body.String() != addUserGoldenOutput {
		fmt.Println(response.Body.String())
		t.Fatalf("Response doesn't match golden response")
	}

	if response.Header().Get("Location") != "/users/1" {
		t.Fatalf("Expected location of '/users/1' got %s",
			response.Header().Get("Location"))
	}
}

func TestDeleteUser(t *testing.T) {
	dbh, m := setupTest(t)
	loadFixtures(dbh)

	users, err := dbh.GetAllUsers()
	failOnError(t, err)

	req, _ := http.NewRequest("DELETE",
		fmt.Sprintf("/api/1/users/%d", users[0].Id), nil)
	response := httptest.NewRecorder()
	m.ServeHTTP(response, req)
	if response.Code != 204 {
		fmt.Println(response.Body.String())
		t.Fatalf("Expected 204 response code, got %d", response.Code)
	}
	_, err = dbh.GetUserById(users[0].Id)
	if err == nil {
		t.Fatalf("Found user when it should have been deleted")
	}
}
