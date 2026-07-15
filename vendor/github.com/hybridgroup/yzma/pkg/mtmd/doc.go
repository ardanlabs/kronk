// Package mtmd provides Go FFI bindings for the mtmd library which is part of
// the llama.cpp project used for multimodal (vision-language) models.
//
// Supported input modalities:
//   - Images: BitmapInit / BitmapInitFromFile / BitmapInitFromBuf
//   - Audio: BitmapInitFromAudio
//   - Video: VideoInit / VideoInitFromBuf (requires ffmpeg/ffprobe on PATH; video
//     support depends on the MTMD_VIDEO compile-time flag in libmtmd)
//
// Use SupportVision, SupportAudio, and SupportVideo to query what the loaded
// projector supports at runtime.
package mtmd
