package gerrit

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_getGerritMessage(t *testing.T) {
	msg, err := GetCommitMessage(context.Background(), "storj%2Fvelero-plugin~master~I6d20b5a8605a99740834df326ad26e646eae206e", "9288388465675dd98e30f30e2575c25d3e9f8880")
	assert.NoError(t, err)
	assert.Contains(t, msg, "The commit contains almost a working")
}
