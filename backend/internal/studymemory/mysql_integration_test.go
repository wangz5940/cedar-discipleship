//go:build integration

package studymemory

import (
	"agp/backend/internal/testdb"
	"testing"
	"time"
)

func TestMySQLStudyMemoryPersistsAndIsolatesAccounts(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "019_study_memory.sql")
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,password_hash,created_at,updated_at) VALUES (11,'one','One','one','hash',NOW(),NOW()),(22,'two','Two','two','hash',NOW(),NOW())`)
	r := NewMySQLRepository(db)
	ctx := t.Context()
	now := time.Now().UTC().UnixMilli()
	if err := r.SaveProgress(ctx, 11, "ovcm:rte/04", Progress{Time: 1182, Duration: 6000, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := r.SetFavorite(ctx, 11, "ovcm:rte/04", &Favorite{Key: "ovcm:rte/04", Kind: "ovcm", Title: "第四课", Type: "audio", CourseID: "rte", LessonID: "04", SavedAt: now}); err != nil {
		t.Fatal(err)
	}
	fresh := NewMySQLRepository(db)
	data, err := fresh.Load(ctx, 11)
	if err != nil || data.Progress["ovcm:rte/04"].Time != 1182 || len(data.Favorites) != 1 {
		t.Fatalf("persisted data: %+v %v", data, err)
	}

	// Repeated favorite writes from another device must also refresh list ordering.
	later := now + 1000
	if err := fresh.SetFavorite(ctx, 11, "ovcm:rte/04", &Favorite{Key: "ovcm:rte/04", Kind: "ovcm", Title: "第四课更新", Type: "audio", CourseID: "rte", LessonID: "04", SavedAt: later}); err != nil {
		t.Fatal(err)
	}
	data, err = fresh.Load(ctx, 11)
	if err != nil || data.Favorites["ovcm:rte/04"].SavedAt != later || data.Favorites["ovcm:rte/04"].Title != "第四课更新" {
		t.Fatalf("favorite update lost timestamp or title: %+v %v", data, err)
	}
	other, err := fresh.Load(ctx, 22)
	if err != nil || len(other.Progress) != 0 || len(other.Favorites) != 0 {
		t.Fatalf("other user: %+v %v", other, err)
	}
	if err := fresh.SaveProgress(ctx, 11, "ovcm:rte/04", Progress{Time: 1200, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := fresh.SetFavorite(ctx, 11, "ovcm:rte/04", nil); err != nil {
		t.Fatal(err)
	}
	data, err = fresh.Load(ctx, 11)
	if err != nil || data.Progress["ovcm:rte/04"].Time != 1200 || len(data.Favorites) != 0 {
		t.Fatalf("updated data: %+v %v", data, err)
	}
}
