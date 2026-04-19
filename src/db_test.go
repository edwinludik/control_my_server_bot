package main

import (
	"testing"
)

//goland:noinspection ALL
func TestUserStore(t *testing.T) {
	store, err := NewUserStoreWithDB(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory store: %v", err)
	}
	defer store.Close()

	userID := int64(456)

	t.Run("AddAndExists", func(t *testing.T) {
		exists, err := store.UserExists(userID)
		if err != nil {
			t.Errorf("UserExists failed: %v", err)
		}
		if exists {
			t.Error("expected user to not exist initially")
		}

		if err := store.AddUser(userID); err != nil {
			t.Fatalf("AddUser failed: %v", err)
		}

		exists, err = store.UserExists(userID)
		if err != nil {
			t.Errorf("UserExists failed: %v", err)
		}
		if !exists {
			t.Error("expected user to exist after AddUser")
		}
	})

	t.Run("ListUsers", func(t *testing.T) {
		ownerID := int64(1)
		users, err := store.ListUsers(ownerID)
		if err != nil {
			t.Fatalf("ListUsers failed: %v", err)
		}

		// ListUsers returns strings like "User: %d"
		foundOwner := false
		foundUser := false
		for _, u := range users {
			if u == "User: 1 (Owner, cannot be deleted)" {
				foundOwner = true
			}
			if u == "User: 456" {
				foundUser = true
			}
		}

		if !foundOwner {
			t.Error("owner not found in user list")
		}
		if !foundUser {
			t.Error("added user not found in user list")
		}
	})

	t.Run("DeleteUser", func(t *testing.T) {
		if err := store.DeleteUser(userID); err != nil {
			t.Fatalf("DeleteUser failed: %v", err)
		}

		exists, err := store.UserExists(userID)
		if err != nil {
			t.Errorf("UserExists failed: %v", err)
		}
		if exists {
			t.Error("expected user to not exist after DeleteUser")
		}
	})
}
