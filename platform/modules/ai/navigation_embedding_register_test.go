package ai

import "testing"

func TestNavigationEmbeddingPayloadAcceptsBatchAndLegacyDocument(t *testing.T) {
	batch := NavigationDocumentsChangedPayload{Documents: []NavigationDocumentRef{{DocumentID: "doc-1", CatalogRevision: 2}, {DocumentID: "doc-2", CatalogRevision: 3}}}
	if refs := batch.documentRefs(); len(refs) != 2 || refs[1].DocumentID != "doc-2" {
		t.Fatalf("unexpected batch refs: %#v", refs)
	}
	legacy := NavigationDocumentsChangedPayload{DocumentID: "legacy", CatalogRevision: 1}
	if refs := legacy.documentRefs(); len(refs) != 1 || refs[0].DocumentID != "legacy" {
		t.Fatalf("legacy payload was not preserved: %#v", refs)
	}
	jobPayload := navigationEmbeddingJobPayload{Documents: batch.Documents, ModelAlias: "embed"}
	if refs := jobPayload.documentRefs(); len(refs) != 2 {
		t.Fatalf("unexpected job refs: %#v", refs)
	}
}

func TestNavigationEmbeddingPayloadRejectsEmptyDocumentSet(t *testing.T) {
	if refs := (NavigationDocumentsChangedPayload{}).documentRefs(); len(refs) != 0 {
		t.Fatalf("unexpected refs: %#v", refs)
	}
}
