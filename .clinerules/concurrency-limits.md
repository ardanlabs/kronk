## Brief overview
This rule establishes concurrency limitations for the Kronk model server and sub-agent execution to ensure proper resource management and system stability.

## Communication style
- When discussing parallel operations, always consider the 2-request server limit
- Acknowledge that sub-agents can run concurrently but must respect server constraints
- Be explicit about concurrency limitations when planning multi-step operations

## Development workflow
- Design sub-agent workflows to avoid exceeding 2 parallel requests to the model server
- Implement proper queuing or throttling when more than 2 concurrent operations are needed
- Monitor concurrent sub-agent execution to ensure compliance with server limits

## Coding best practices
- When implementing parallel operations, implement concurrency control to maintain ≤2 simultaneous requests to the model server
- Use semaphores or similar mechanisms to limit concurrent sub-agent execution to 2 at a time
- Document any concurrency limitations in code comments and API documentation

## Project context
- The model server has a hard limit of 2 parallel requests at any given time
- Sub-agents can be launched in parallel, but must be managed to not exceed this server limit
- This constraint affects how multi-step processes are designed and executed

## Other guidelines
- When planning multi-agent workflows, always account for the 2-request server limit
- Implement proper error handling for cases where the concurrency limit is approached
- Test concurrent operations to ensure they respect the server's 2-request maximum