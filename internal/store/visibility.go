package store

const visibleTrackPredicate = `(t.source_peer <> '' OR EXISTS (
	SELECT 1 FROM local_library ll WHERE ll.id = t.library_id AND ll.enabled != 0
))`
