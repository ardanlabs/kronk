package llama

import (
	"errors"
	"unsafe"

	"github.com/jupiterrider/ffi"
)

var (
	// LLAMA_API void llama_memory_clear(
	// 				llama_memory_t mem,
	// 				bool data);
	memoryClearFunc ffi.Fun

	// LLAMA_API bool llama_memory_seq_rm(
	//         		llama_memory_t mem,
	//           	llama_seq_id seq_id,
	//              llama_pos p0,
	//              llama_pos p1);
	memorySeqRmFunc ffi.Fun

	// LLAMA_API void llama_memory_seq_cp(...)
	memorySeqCpFunc ffi.Fun

	// LLAMA_API void llama_memory_seq_keep(...)
	memorySeqKeepFunc ffi.Fun

	// LLAMA_API void llama_memory_seq_add(...)
	memorySeqAddFunc ffi.Fun

	// LLAMA_API void llama_memory_seq_div(...)
	memorySeqDivFunc ffi.Fun

	// LLAMA_API llama_pos llama_memory_seq_pos_min(...)
	memorySeqPosMinFunc ffi.Fun

	// LLAMA_API llama_pos llama_memory_seq_pos_max(...)
	memorySeqPosMaxFunc ffi.Fun

	// LLAMA_API bool llama_memory_can_shift(...)
	memoryCanShiftFunc ffi.Fun
)

func loadMemoryFuncs(lib ffi.Lib) error {
	var err error

	if memoryClearFunc, err = lib.Prep("llama_memory_clear", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeUint8); err != nil {
		return loadError("llama_memory_clear", err)
	}

	if memorySeqRmFunc, err = lib.Prep("llama_memory_seq_rm", &ffi.TypeUint8, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("llama_memory_seq_rm", err)
	}

	if memorySeqCpFunc, err = lib.Prep("llama_memory_seq_cp", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("llama_memory_seq_cp", err)
	}

	if memorySeqKeepFunc, err = lib.Prep("llama_memory_seq_keep", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_memory_seq_keep", err)
	}

	if memorySeqAddFunc, err = lib.Prep("llama_memory_seq_add", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("llama_memory_seq_add", err)
	}

	if memorySeqDivFunc, err = lib.Prep("llama_memory_seq_div", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("llama_memory_seq_div", err)
	}

	if memorySeqPosMinFunc, err = lib.Prep("llama_memory_seq_pos_min", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_memory_seq_pos_min", err)
	}

	if memorySeqPosMaxFunc, err = lib.Prep("llama_memory_seq_pos_max", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_memory_seq_pos_max", err)
	}

	if memoryCanShiftFunc, err = lib.Prep("llama_memory_can_shift", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("llama_memory_can_shift", err)
	}

	return nil
}

var (
	errInvalidMemory = errors.New("invalid memory handle")
)

// MemoryClear clears the memory contents.
// If data == true, the data buffers will also be cleared together with the metadata.
func MemoryClear(mem Memory, data bool) error {
	if mem == 0 {
		return errInvalidMemory
	}
	memoryClearFunc.Call(nil, unsafe.Pointer(&mem), &data)
	return nil
}

// MemorySeqRm removes all tokens that belong to the specified sequence and have positions in [p0, p1).
// Returns false if a partial sequence cannot be removed. Removing a whole sequence never fails.
// seqID < 0 : match any sequence
// p0 < 0     : [0,  p1]
// p1 < 0     : [p0, inf)
func MemorySeqRm(mem Memory, seqID SeqId, p0, p1 Pos) (bool, error) {
	if mem == 0 {
		return false, errInvalidMemory
	}
	var result ffi.Arg
	memorySeqRmFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&mem), &seqID, &p0, &p1)

	return result.Bool(), nil
}

// MemorySeqCp copies all tokens from one sequence to another.
func MemorySeqCp(mem Memory, seqIDSrc, seqIDDst SeqId, p0, p1 Pos) error {
	if mem == 0 {
		return errInvalidMemory
	}
	memorySeqCpFunc.Call(nil, unsafe.Pointer(&mem), &seqIDSrc, &seqIDDst, &p0, &p1)
	return nil
}

// MemorySeqKeep removes all tokens that do not belong to the specified sequence.
func MemorySeqKeep(mem Memory, seqID SeqId) error {
	if mem == 0 {
		return errInvalidMemory
	}
	memorySeqKeepFunc.Call(nil, unsafe.Pointer(&mem), &seqID)
	return nil
}

// MemorySeqAdd adds a relative position delta to tokens in the specified sequence and range.
func MemorySeqAdd(mem Memory, seqID SeqId, p0, p1, delta Pos) error {
	if mem == 0 {
		return errInvalidMemory
	}
	memorySeqAddFunc.Call(nil, unsafe.Pointer(&mem), &seqID, &p0, &p1, &delta)
	return nil
}

// MemorySeqDiv divides the positions of tokens in the specified sequence and range by a factor.
func MemorySeqDiv(mem Memory, seqID SeqId, p0, p1 Pos, d int) error {
	if mem == 0 {
		return errInvalidMemory
	}
	memorySeqDivFunc.Call(nil, unsafe.Pointer(&mem), &seqID, &p0, &p1, &d)
	return nil
}

// MemorySeqPosMin returns the smallest position in the memory for the specified sequence.
func MemorySeqPosMin(mem Memory, seqID SeqId) (Pos, error) {
	if mem == 0 {
		return 0, errInvalidMemory
	}
	var result ffi.Arg
	memorySeqPosMinFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&mem), &seqID)
	return Pos(int32(result)), nil
}

// MemorySeqPosMax returns the largest position in the memory for the specified sequence.
func MemorySeqPosMax(mem Memory, seqID SeqId) (Pos, error) {
	if mem == 0 {
		return 0, errInvalidMemory
	}
	var result ffi.Arg
	memorySeqPosMaxFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&mem), &seqID)
	return Pos(int32(result)), nil
}

// MemoryCanShift checks if the memory supports shifting.
func MemoryCanShift(mem Memory) (bool, error) {
	if mem == 0 {
		return false, errInvalidMemory
	}
	var result ffi.Arg
	memoryCanShiftFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&mem))
	return result.Bool(), nil
}
