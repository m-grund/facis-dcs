package c2pa

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJUMBFBoxWriter_ProducesValidHeader(t *testing.T) {
	payload := []byte("hello jumbf")
	tbox := [4]byte{'t', 'e', 's', 't'}
	box := WriteBox(tbox, payload)

	// Header must be 8 bytes: 4-byte LBox + 4-byte TBox.
	assert.GreaterOrEqual(t, len(box), 8)

	totalLen := binary.BigEndian.Uint32(box[0:4])
	assert.Equal(t, uint32(8+len(payload)), totalLen)

	assert.Equal(t, tbox[:], box[4:8])
}

func TestJUMBFBoxWriter_ContentPreserved(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03, 0xAB, 0xCD}
	tbox := [4]byte{'j', 's', 'o', 'n'}
	box := WriteBox(tbox, payload)

	assert.Equal(t, payload, box[8:])
}

func TestJUMBFBoxWriter_EmptyPayload(t *testing.T) {
	tbox := [4]byte{'c', 'b', 'o', 'r'}
	box := WriteBox(tbox, nil)
	assert.Equal(t, 8, len(box))
	totalLen := binary.BigEndian.Uint32(box[0:4])
	assert.Equal(t, uint32(8), totalLen)
}

func TestWriteJSONBox_WrapsCorrectly(t *testing.T) {
	data := []byte(`{"key":"value"}`)
	box := WriteJSONBox(data)
	// TBox must be 'json'.
	assert.Equal(t, []byte{'j', 's', 'o', 'n'}, box[4:8])
	assert.Equal(t, data, box[8:])
}

func TestWriteCBORBox_WrapsCorrectly(t *testing.T) {
	data := []byte{0x81, 0x01}
	box := WriteCBORBox(data)
	assert.Equal(t, []byte{'c', 'b', 'o', 'r'}, box[4:8])
	assert.Equal(t, data, box[8:])
}

func TestWriteDescriptionBox_ContainsUUIDAndLabel(t *testing.T) {
	uuid := c2paManifestUUID
	label := "c2pa.manifest"
	box := WriteDescriptionBox(uuid, label, 0x03)

	// TBox must be 'jumd'.
	assert.Equal(t, []byte{'j', 'u', 'm', 'd'}, box[4:8])

	// Content: UUID(16) + toggles(1) + label + NUL.
	content := box[8:]
	assert.Equal(t, uuid[:], content[:16])
	assert.Equal(t, byte(0x03), content[16])
	assert.True(t, bytes.Contains(content, []byte(label)))
	// Null terminator.
	assert.Equal(t, byte(0x00), content[len(content)-1])
}

func TestWriteSuperbox_ContainsDescriptionAndChildren(t *testing.T) {
	uuid := c2paAssertionUUID
	label := "test.box"
	child1 := WriteJSONBox([]byte(`{}`))
	child2 := WriteCBORBox([]byte{0x80})

	super := WriteSuperbox(uuid, label, child1, child2)

	// Outer box TBox must be 'jumb'.
	assert.Equal(t, []byte{'j', 'u', 'm', 'b'}, super[4:8])

	// Content must include label.
	assert.True(t, bytes.Contains(super, []byte(label)))

	// Content must include child bytes.
	assert.True(t, bytes.Contains(super, child1))
	assert.True(t, bytes.Contains(super, child2))
}
