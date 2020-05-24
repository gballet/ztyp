package view

import (
	"encoding/binary"
	"fmt"
	. "github.com/protolambda/ztyp/tree"
	"io"
)

const OffsetByteLength = 4

type ComplexTypeBase struct {
	MinSize     uint64
	MaxSize     uint64
	Size        uint64
	IsFixedSize bool
}

func (td *ComplexTypeBase) IsFixedByteLength() bool {
	return td.IsFixedSize
}

func (td *ComplexTypeBase) TypeByteLength() uint64 {
	return td.Size
}

func (td *ComplexTypeBase) MinByteLength() uint64 {
	return td.MinSize
}

func (td *ComplexTypeBase) MaxByteLength() uint64 {
	return td.MaxSize
}

func (td *ComplexTypeBase) checkScope(scope uint64) error {
	if scope < td.MinSize {
		return fmt.Errorf("scope %d is too small, need at least %d bytes", scope, td.MinSize)
	}
	if scope > td.MaxSize {
		return fmt.Errorf("scope %d is too big, need %d or less bytes", scope, td.MaxSize)
	}
	return nil
}

type VectorTypeDef interface {
	TypeDef
	ElementType() TypeDef
	Length() uint64
}

func VectorType(elemType TypeDef, length uint64) VectorTypeDef {
	basicElemType, ok := elemType.(BasicTypeDef)
	if ok {
		return BasicVectorType(basicElemType, length)
	} else {
		return ComplexVectorType(elemType, length)
	}
}

type ListTypeDef interface {
	TypeDef
	ElementType() TypeDef
	Limit() uint64
}

func ListType(elemType TypeDef, limit uint64) ListTypeDef {
	basicElemType, ok := elemType.(BasicTypeDef)
	if ok {
		return BasicListType(basicElemType, limit)
	} else {
		return ComplexListType(elemType, limit)
	}
}

type ErrNodeIter struct {
	error
}

func (e ErrNodeIter) Next() (chunk Node, ok bool, err error) {
	return nil, false, e.error
}

type NodeIterFn func() (chunk Node, ok bool, err error)

func (f NodeIterFn) Next() (chunk Node, ok bool, err error) {
	return f()
}

type NodeIter interface {
	// Next gets the next node, ok is true if it actually exists.
	// An error may occur if data is missing or corrupt.
	Next() (chunk Node, ok bool, err error)
}

type ErrElemIter struct {
	error
}

func (e ErrElemIter) Next() (elem View, ok bool, err error) {
	return nil, false, e.error
}

type ElemIterFn func() (elem View, ok bool, err error)

func (f ElemIterFn) Next() (elem View, ok bool, err error) {
	return f()
}

type ElemIter interface {
	// Next gets the next element, ok is true if it actually exists.
	// An error may occur if data is missing or corrupt.
	Next() (elem View, ok bool, err error)
}

func WriteOffset(w io.Writer, prevOffset uint64, elemLen uint64) (offset uint64, err error) {
	if prevOffset >= (uint64(1) << 32) {
		panic("cannot write offset with invalid previous offset")
	}
	if elemLen >= (uint64(1) << 32) {
		panic("cannot write offset with invalid element size")
	}
	offset = prevOffset + elemLen
	if offset >= (uint64(1) << 32) {
		panic("offset too large, not uint32")
	}
	tmp := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(tmp, uint32(offset))
	_, err = w.Write(tmp)
	return
}

func ReadOffset(r io.Reader) (uint32, error) {
	tmp := make([]byte, 4, 4)
	_, err := r.Read(tmp)
	return binary.LittleEndian.Uint32(tmp), err
}

func serializeComplexFixElemSeries(iter ElemIter, w io.Writer) error {
	for {
		el, ok, err := iter.Next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		if err := el.Serialize(w); err != nil {
			return err
		}
	}
	return nil
}

func serializeComplexVarElemSeries(length uint64, iterFn func() ElemIter, w io.Writer) error {
	// the previous offset, to calculate a new offset from, starting after the fixed data.
	prevOffset := length * OffsetByteLength

	// span of the previous var-size element
	prevSize := uint64(0)
	iter := iterFn()
	// write all offsets
	for {
		el, ok, err := iter.Next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		elValSize, err := el.ValueByteLength()
		if err != nil {
			return err
		}
		prevOffset, err = WriteOffset(w, prevOffset, prevSize)
		if err != nil {
			return err
		}
		prevSize = elValSize
	}
	iter = iterFn()
	// now write all elements
	for {
		el, ok, err := iter.Next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		if err := el.Serialize(w); err != nil {
			return err
		}
	}
	return nil
}
