package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/andybarilla/exit66jukebox/internal/store"
)

// adminGroups calls one group endpoint as an admin and returns the group list
// every one of them answers with. The cookie is made once per test: adminReq
// creates a fresh admin user each call, which collides on the second.
func adminGroups(t *testing.T, s *Server, cookie *http.Cookie, method, path, body string) []store.FederationGroup {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s = %d (%s)", method, path, rec.Code, rec.Body)
	}
	var resp struct {
		Groups []store.FederationGroup `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode groups: %v", err)
	}
	return resp.Groups
}

func TestAdminFederationGroupLifecycle(t *testing.T) {
	s, db := newTestServer(t)
	_, cookie := adminSessionWithEmail(t, db, "groups-admin@b.com")

	groups := adminGroups(t, s, cookie, "POST", "/api/admin/federation/groups", `{"name":"family"}`)
	if len(groups) != 1 || groups[0].Name != "family" {
		t.Fatalf("after create: %#v", groups)
	}
	id := strconv.FormatInt(groups[0].ID, 10)

	groups = adminGroups(t, s, cookie, "POST", "/api/admin/federation/groups/"+id+"/members", `{"peer_id":"office"}`)
	if len(groups[0].Members) != 1 || groups[0].Members[0] != "office" {
		t.Fatalf("after add member: %#v", groups)
	}
	// The membership the catalog check reads is the one the endpoint wrote.
	visible, err := store.FederationCatalogVisible(db, "office", "home")
	if err != nil {
		t.Fatal(err)
	}
	if visible {
		t.Fatal("home is not in the group yet, so office must not be visible to it")
	}
	adminGroups(t, s, cookie, "POST", "/api/admin/federation/groups/"+id+"/members", `{"peer_id":"home"}`)
	if visible, err = store.FederationCatalogVisible(db, "office", "home"); err != nil || !visible {
		t.Fatalf("both peers are in family now: visible = %v, err = %v", visible, err)
	}

	groups = adminGroups(t, s, cookie, "DELETE", "/api/admin/federation/groups/"+id+"/members/office", "")
	if len(groups[0].Members) != 1 || groups[0].Members[0] != "home" {
		t.Fatalf("after remove member: %#v", groups)
	}

	groups = adminGroups(t, s, cookie, "DELETE", "/api/admin/federation/groups/"+id, "")
	if len(groups) != 0 {
		t.Fatalf("after delete: %#v", groups)
	}
}

// Every group route, under both callers that must be refused. Routing is
// uniformly requireAdmin, so this is a guard against one route being added
// without it rather than a claim that they differ.
func TestAdminFederationGroupsRequireAdmin(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/federation/groups", ""},
		{http.MethodPost, "/api/admin/federation/groups", `{"name":"family"}`},
		{http.MethodDelete, "/api/admin/federation/groups/1", ""},
		{http.MethodPost, "/api/admin/federation/groups/1/members", `{"peer_id":"office"}`},
		{http.MethodDelete, "/api/admin/federation/groups/1/members/office", ""},
	}
	for _, route := range routes {
		for _, caller := range []string{"anonymous", "non-admin"} {
			t.Run(route.method+" "+route.path+" as "+caller, func(t *testing.T) {
				s, db := newTestServer(t)
				// A group to act on, so a refusal cannot be mistaken for a 404.
				g, err := store.CreateFederationGroup(db, "family")
				if err != nil {
					t.Fatal(err)
				}
				if err := store.AddFederationGroupMember(db, g.ID, "office"); err != nil {
					t.Fatal(err)
				}

				req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				if caller == "non-admin" {
					req.AddCookie(nonAdminCookie(t, db))
				}
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, req)

				if rec.Code == http.StatusOK {
					t.Fatalf("%s %s answered 200 to a %s caller", route.method, route.path, caller)
				}
				// And it must not have acted: the group and its member survive.
				groups, err := store.ListFederationGroups(db)
				if err != nil {
					t.Fatal(err)
				}
				if len(groups) != 1 || len(groups[0].Members) != 1 {
					t.Fatalf("a %s caller changed the groups: %+v", caller, groups)
				}
			})
		}
	}
}
