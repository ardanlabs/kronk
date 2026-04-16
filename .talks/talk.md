## Kronk: Hardware accelerated local inference

### About this Session

In this talk Bill will introduce Kronk, a new SDK that allows you to write AI based apps without the need of a model server. If you have Apple Metal (Mac), CUDA (NVIDIA), or Vulkan, Kronk can tap into that GPU power instead of grinding through the work on the CPU alone.

To dog food the SDK, Bill wrote a model server (KMS) that is optimized to run your local AI workloads with performance in mind. During the talk, Bill will show how you can use Agents like Kilo Code to run local agentic workloads to perform basic work.

### Outline

- Introduction
  - Who am I and why I built Kronk
  - What is Kronk
  - How local inference is the future
- Angentic Use
  - Running KMS and Kilo Code
  - Ask for code summaries and git summaries
- Build Tic-Tac-Toe App
  - Go service with TUI front end
  - Use local model for player 2
