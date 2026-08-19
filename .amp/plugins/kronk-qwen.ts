// @amp-agent-mode {"key":"kronk-qwen38","label":"Kronk Qwen"}

/*
Setup for later testing:

1. Protect Kronk before exposing it by setting this in model_config.yaml and
   restarting the server:

   kms:
     authorization:
       mode: full-protected

2. Create an application token with the inference grants Amp may use:

   export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
   kronk security token create \
     --duration 720h \
     --endpoints chat-completions,responses,messages

3. Expose http://localhost:11435 through an authenticated HTTPS reverse proxy
   or tunnel. Amp's inference backend cannot connect directly to localhost.

4. Register the HTTPS endpoint with Amp, using the application token produced
   above:

   export AMP_KRONK_TOKEN='<application-token>'
   amp config model-providers add-router openai-compatible \
     --name kronk \
     --models 'unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT' \
     --api-key-env AMP_KRONK_TOKEN \
     --base-url 'https://your-kronk-host.example/v1' \
     --active

5. Start a new Amp thread and select "Kronk Qwen" from the mode picker.

This plugin only registers the mode. It does not contact Kronk or affect other
modes until "Kronk Qwen" is selected. Oracle and Librarian retain Amp's
normal routing.
*/

import type {PluginAPI} from '@ampcode/plugin';

export const description = 'Adds an Amp agent mode backed by the Kronk Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT model.';

export default function (amp: PluginAPI) {
    const agent = amp.createAgent({
        model: 'unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT',
        instructions: 'Work as the main Amp coding agent. Follow the project guidance and use the available tools when they improve the result.',
        tools: 'all',
        display: {
            label: 'Kronk Qwen',
        },
    });

    amp.registerAgentMode({
        key: 'kronk-qwen',
        description: 'Uses the local Kronk-hosted Qwen3.6 MTP 35B-A3B UD-Q8_K_XL model as the main coding agent. Use to run agent work through Kronk while retaining Amp tools.',
        agent: agent.definition,
    });
}
