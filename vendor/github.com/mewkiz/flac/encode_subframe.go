package flac

import (
	"fmt"

	"github.com/icza/bitio"
	"github.com/mewkiz/flac/frame"
	iobits "github.com/mewkiz/flac/internal/bits"
	"github.com/mewkiz/pkg/errutil"
)

// --- [ Subframe ] ------------------------------------------------------------

// encodeSubframe encodes the given subframe, writing to bw.
func encodeSubframe(bw *bitio.Writer, hdr frame.Header, subframe *frame.Subframe, bps uint) error {
	// Encode subframe header.
	if err := encodeSubframeHeader(bw, subframe.SubHeader); err != nil {
		return errutil.Err(err)
	}

	// Adjust bps of subframe for wasted bits-per-sample.
	bps -= subframe.Wasted

	// Right shift to account for wasted bits-per-sample.
	if subframe.Wasted > 0 {
		for i, sample := range subframe.Samples {
			subframe.Samples[i] = sample >> subframe.Wasted
		}
		// NOTE: use defer to restore original samples after encode.
		defer func() {
			for i, sample := range subframe.Samples {
				subframe.Samples[i] = sample << subframe.Wasted
			}
		}()
	}

	// Encode audio samples.
	switch subframe.Pred {
	case frame.PredConstant:
		if err := encodeConstantSamples(bw, hdr, subframe, bps); err != nil {
			return errutil.Err(err)
		}
	case frame.PredVerbatim:
		if err := encodeVerbatimSamples(bw, hdr, subframe, bps); err != nil {
			return errutil.Err(err)
		}
	case frame.PredFixed:
		if err := encodeFixedSamples(bw, hdr, subframe, bps); err != nil {
			return errutil.Err(err)
		}
	case frame.PredFIR:
		if err := encodeFIRSamples(bw, hdr, subframe, bps); err != nil {
			return errutil.Err(err)
		}
	default:
		return errutil.Newf("support for prediction method %v not yet implemented", subframe.Pred)
	}
	return nil
}

// --- [ Subframe header ] -----------------------------------------------------

// encodeSubframeHeader encodes the given subframe header, writing to bw.
func encodeSubframeHeader(bw *bitio.Writer, subHdr frame.SubHeader) error {
	// Zero bit padding, to prevent sync-fooling string of 1s.
	if err := bw.WriteBits(0x0, 1); err != nil {
		return errutil.Err(err)
	}

	// Subframe type:
	//     000000 : SUBFRAME_CONSTANT
	//     000001 : SUBFRAME_VERBATIM
	//     00001x : reserved
	//     0001xx : reserved
	//     001xxx : if(xxx <= 4) SUBFRAME_FIXED, xxx=order ; else reserved
	//     01xxxx : reserved
	//     1xxxxx : SUBFRAME_LPC, xxxxx=order-1
	var bits uint64
	switch subHdr.Pred {
	case frame.PredConstant:
		// 000000 : SUBFRAME_CONSTANT
		bits = 0x00
	case frame.PredVerbatim:
		// 000001 : SUBFRAME_VERBATIM
		bits = 0x01
	case frame.PredFixed:
		// 001xxx : if(xxx <= 4) SUBFRAME_FIXED, xxx=order ; else reserved
		bits = 0x08 | uint64(subHdr.Order)
	case frame.PredFIR:
		// 1xxxxx : SUBFRAME_LPC, xxxxx=order-1
		bits = 0x20 | uint64(subHdr.Order-1)
	}
	if err := bw.WriteBits(bits, 6); err != nil {
		return errutil.Err(err)
	}

	// <1+k> 'Wasted bits-per-sample' flag:
	//
	//     0 : no wasted bits-per-sample in source subblock, k=0
	//     1 : k wasted bits-per-sample in source subblock, k-1 follows, unary coded; e.g. k=3 => 001 follows, k=7 => 0000001 follows.
	hasWastedBits := subHdr.Wasted > 0
	if err := bw.WriteBool(hasWastedBits); err != nil {
		return errutil.Err(err)
	}
	if hasWastedBits {
		if err := iobits.WriteUnary(bw, uint64(subHdr.Wasted-1)); err != nil {
			return errutil.Err(err)
		}
	}
	return nil
}

// --- [ Constant samples ] ----------------------------------------------------

// encodeConstantSamples stores the given constant sample, writing to bw.
func encodeConstantSamples(bw *bitio.Writer, hdr frame.Header, subframe *frame.Subframe, bps uint) error {
	samples := subframe.Samples
	sample := samples[0]
	for _, s := range samples[1:] {
		if sample != s {
			return errutil.Newf("constant sample mismatch; expected %v, got %v", sample, s)
		}
	}
	// Unencoded constant value of the subblock, n = frame's bits-per-sample.
	if err := bw.WriteBits(uint64(sample), uint8(bps)); err != nil {
		return errutil.Err(err)
	}
	return nil
}

// --- [ Verbatim samples ] ----------------------------------------------------

// encodeVerbatimSamples stores the given samples verbatim (uncompressed),
// writing to bw.
func encodeVerbatimSamples(bw *bitio.Writer, hdr frame.Header, subframe *frame.Subframe, bps uint) error {
	// Unencoded subblock; n = frame's bits-per-sample, i = frame's blocksize.
	samples := subframe.Samples
	if int(hdr.BlockSize) != len(samples) {
		return errutil.Newf("block size and sample count mismatch; expected %d, got %d", hdr.BlockSize, len(samples))
	}
	for _, sample := range samples {
		if err := bw.WriteBits(uint64(sample), uint8(bps)); err != nil {
			return errutil.Err(err)
		}
	}
	return nil
}

// --- [ Fixed samples ] -------------------------------------------------------

// encodeFixedSamples stores the given samples using linear prediction coding
// with a fixed set of predefined polynomial coefficients, writing to bw.
func encodeFixedSamples(bw *bitio.Writer, hdr frame.Header, subframe *frame.Subframe, bps uint) error {
	// Encode unencoded warm-up samples.
	samples := subframe.Samples
	for i := 0; i < subframe.Order; i++ {
		sample := samples[i]
		if err := bw.WriteBits(uint64(sample), uint8(bps)); err != nil {
			return errutil.Err(err)
		}
	}

	// Compute residuals (signal errors of the prediction) between audio
	// samples and LPC predicted audio samples.
	const shift = 0
	residuals, err := getLPCResiduals(subframe, frame.FixedCoeffs[subframe.Order], shift)
	if err != nil {
		return errutil.Err(err)
	}

	// Encode subframe residuals.
	if err := encodeResiduals(bw, subframe, residuals); err != nil {
		return errutil.Err(err)
	}
	return nil
}

// --- [ FIR samples ] -------------------------------------------------------

// encodeFIRSamples stores the given samples using linear prediction coding
// with a custom set of predefined polynomial coefficients, writing to bw.
func encodeFIRSamples(bw *bitio.Writer, hdr frame.Header, subframe *frame.Subframe, bps uint) error {
	// Encode unencoded warm-up samples.
	samples := subframe.Samples
	for i := 0; i < subframe.Order; i++ {
		sample := samples[i]
		if err := bw.WriteBits(uint64(sample), uint8(bps)); err != nil {
			return errutil.Err(err)
		}
	}

	// 4 bits: (coefficients' precision in bits) - 1.
	if err := bw.WriteBits(uint64(subframe.CoeffPrec-1), 4); err != nil {
		return errutil.Err(err)
	}

	// 5 bits: predictor coefficient shift needed in bits.
	if err := bw.WriteBits(uint64(subframe.CoeffShift), 5); err != nil {
		return errutil.Err(err)
	}

	// Encode coefficients.
	for _, coeff := range subframe.Coeffs {
		// (prec) bits: Predictor coefficient.
		if err := bw.WriteBits(uint64(coeff), uint8(subframe.CoeffPrec)); err != nil {
			return errutil.Err(err)
		}
	}

	// Compute residuals (signal errors of the prediction) between audio
	// samples and LPC predicted audio samples.
	residuals, err := getLPCResiduals(subframe, subframe.Coeffs, subframe.CoeffShift)
	if err != nil {
		return errutil.Err(err)
	}

	// Encode subframe residuals.
	if err := encodeResiduals(bw, subframe, residuals); err != nil {
		return errutil.Err(err)
	}
	return nil
}

// encodeResiduals encodes the residuals (prediction method error signals) of the
// subframe.
//
// ref: https://www.xiph.org/flac/format.html#residual
func encodeResiduals(bw *bitio.Writer, subframe *frame.Subframe, residuals []int32) error {
	// 2 bits: Residual coding method.
	if err := bw.WriteBits(uint64(subframe.ResidualCodingMethod), 2); err != nil {
		return errutil.Err(err)
	}
	// The 2 bits are used to specify the residual coding method as follows:
	//    00: Rice coding with a 4-bit Rice parameter.
	//    01: Rice coding with a 5-bit Rice parameter.
	//    10: reserved.
	//    11: reserved.
	switch subframe.ResidualCodingMethod {
	case frame.ResidualCodingMethodRice1:
		return encodeRicePart(bw, subframe, 4, residuals)
	case frame.ResidualCodingMethodRice2:
		return encodeRicePart(bw, subframe, 5, residuals)
	default:
		return fmt.Errorf("encodeResiduals: reserved residual coding method bit pattern (%02b)", uint8(subframe.ResidualCodingMethod))
	}
}

// encodeRicePart encodes a Rice partition of residuals from the subframe, using
// a Rice parameter of the specified size in bits.
//
// ref: https://www.xiph.org/flac/format.html#partitioned_rice
// ref: https://www.xiph.org/flac/format.html#partitioned_rice2
func encodeRicePart(bw *bitio.Writer, subframe *frame.Subframe, paramSize uint, residuals []int32) error {
	// 4 bits: Partition order.
	riceSubframe := subframe.RiceSubframe
	if err := bw.WriteBits(uint64(riceSubframe.PartOrder), 4); err != nil {
		return errutil.Err(err)
	}

	// Parse Rice partitions; in total 2^partOrder partitions.
	//
	// ref: https://www.xiph.org/flac/format.html#rice_partition
	// ref: https://www.xiph.org/flac/format.html#rice2_partition
	partOrder := riceSubframe.PartOrder
	nparts := 1 << partOrder
	curResidualIndex := 0
	for i := range riceSubframe.Partitions {
		partition := &riceSubframe.Partitions[i]
		// (4 or 5) bits: Rice parameter.
		param := partition.Param
		if err := bw.WriteBits(uint64(param), uint8(paramSize)); err != nil {
			return errutil.Err(err)
		}

		// Determine the number of Rice encoded samples in the partition.
		var nsamples int
		if partOrder == 0 {
			nsamples = subframe.NSamples - subframe.Order
		} else if i != 0 {
			nsamples = subframe.NSamples / nparts
		} else {
			nsamples = subframe.NSamples/nparts - subframe.Order
		}

		if paramSize == 4 && param == 0xF || paramSize == 5 && param == 0x1F {
			// 1111 or 11111: Escape code, meaning the partition is in unencoded
			// binary form using n bits per sample; n follows as a 5-bit number.
			if err := bw.WriteBits(uint64(partition.EscapedBitsPerSample), 5); err != nil {
				return errutil.Err(err)
			}
			for j := 0; j < nsamples; j++ {
				// ref: https://datatracker.ietf.org/doc/draft-ietf-cellar-flac/
				//
				// From section 9.2.7.1.  Escaped partition:
				//
				// The residual samples themselves are stored signed two's
				// complement.  For example, when a partition is escaped and each
				// residual sample is stored with 3 bits, the number -1 is
				// represented as 0b111.
				residual := residuals[curResidualIndex]
				curResidualIndex++
				if err := bw.WriteBits(uint64(residual), uint8(partition.EscapedBitsPerSample)); err != nil {
					return errutil.Err(err)
				}
			}
			continue
		}

		// Encode the Rice residuals of the partition.
		for j := 0; j < nsamples; j++ {
			residual := residuals[curResidualIndex]
			curResidualIndex++
			if err := encodeRiceResidual(bw, param, residual); err != nil {
				return errutil.Err(err)
			}
		}
	}

	return nil
}

// encodeRiceResidual encodes a Rice residual (error signal).
func encodeRiceResidual(bw *bitio.Writer, k uint, residual int32) error {
	// ZigZag encode.
	folded := iobits.EncodeZigZag(residual)

	// unfold into low- and high.
	lowMask := ^uint32(0) >> (32 - k) // lower k bits.
	highMask := ^uint32(0) << k       // upper bits.
	high := (folded & highMask) >> k
	low := folded & lowMask

	// Write unary encoded most significant bits.
	if err := iobits.WriteUnary(bw, uint64(high)); err != nil {
		return errutil.Err(err)
	}

	// Write binary encoded least significant bits.
	if err := bw.WriteBits(uint64(low), uint8(k)); err != nil {
		return errutil.Err(err)
	}
	return nil
}

// getLPCResiduals returns the residuals (signal errors of the prediction)
// between the given audio samples and the LPC predicted audio samples, using
// the coefficients of a given polynomial, and a couple (order of polynomial;
// i.e. len(coeffs)) of unencoded warm-up samples.
func getLPCResiduals(subframe *frame.Subframe, coeffs []int32, shift int32) ([]int32, error) {
	if len(coeffs) != subframe.Order {
		return nil, fmt.Errorf("getLPCResiduals: prediction order (%d) differs from number of coefficients (%d)", subframe.Order, len(coeffs))
	}
	if shift < 0 {
		return nil, fmt.Errorf("getLPCResiduals: invalid negative shift")
	}
	if subframe.NSamples != len(subframe.Samples) {
		return nil, fmt.Errorf("getLPCResiduals: subframe sample count mismatch; expected %d, got %d", subframe.NSamples, len(subframe.Samples))
	}
	var residuals []int32
	for i := subframe.Order; i < subframe.NSamples; i++ {
		var sample int64
		for j, c := range coeffs {
			sample += int64(c) * int64(subframe.Samples[i-j-1])
		}
		residual := subframe.Samples[i] - int32(sample>>uint(shift))
		residuals = append(residuals, residual)
	}
	return residuals, nil
}
