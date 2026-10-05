# ==============================================================================
# Examples

function Invoke-GoExample {
    param(
        [Parameter(Mandatory)]
        [string]$ExamplePath,

        [string[]]$AdditionalArguments = @()
    )

    Invoke-InDirectory -Path $ExamplesDirectory -Action {
        go run $ExamplePath @AdditionalArguments
        Assert-NativeCommandSucceeded -Command "go run $ExamplePath"
    }
}

# ./pwr.ps1 example-agent
function Invoke-AgentExample {
    Invoke-GoExample -ExamplePath "./agent/..."
}

# ./pwr.ps1 example-audio
function Invoke-AudioExample {
    Invoke-GoExample -ExamplePath "./audio/main.go"
}

# ./pwr.ps1 example-bucky
function Invoke-BuckyExample {
    Invoke-GoExample -ExamplePath "./bucky/main.go"
}

# ./pwr.ps1 example-bucky-stream -Arguments "--silero-vad", "--vad-threshold=0.5"
function Invoke-BuckyStreamExample {
    param([string[]]$ExampleArguments = @())

    Invoke-GoExample -ExamplePath "./bucky-stream/main.go" -AdditionalArguments $ExampleArguments
}

# ./pwr.ps1 example-bucky-stream-vad
function Invoke-BuckyStreamVadExample {
    Invoke-GoExample `
        -ExamplePath "./bucky-stream/main.go" `
        -AdditionalArguments @("--silero-vad", "--vad-threshold=0.5")
}

# ./pwr.ps1 example-bucky-diar
function Invoke-BuckyDiarizationExample {
    Invoke-GoExample -ExamplePath "./bucky-diar/main.go"
}

# ./pwr.ps1 example-chat
function Invoke-ChatExample {
    Invoke-GoExample -ExamplePath "./chat/main.go"
}

# ./pwr.ps1 example-concurrency
function Invoke-ConcurrencyExample {
    Invoke-GoExample -ExamplePath "./concurrency/main.go"
}

# ./pwr.ps1 example-embedding
function Invoke-EmbeddingExample {
    Invoke-GoExample -ExamplePath "./embedding/main.go"
}

# ./pwr.ps1 example-grammar
function Invoke-GrammarExample {
    Invoke-GoExample -ExamplePath "./grammar/main.go"
}

# ./pwr.ps1 example-malina
function Invoke-MalinaExample {
    Invoke-GoExample -ExamplePath "./malina/main.go"
}

# ./pwr.ps1 example-malina-img2img
function Invoke-MalinaImageToImageExample {
    Invoke-GoExample -ExamplePath "./malina-img2img/main.go"
}

# ./pwr.ps1 example-malina-sd-encode
function Invoke-MalinaSdEncodeExample {
    Invoke-GoExample -ExamplePath "./malina-sd-encode/main.go"
}

# ./pwr.ps1 example-malina-system
function Invoke-MalinaSystemExample {
    Invoke-GoExample -ExamplePath "./malina-system/main.go"
}

# ./pwr.ps1 example-malina-controlnet
function Invoke-MalinaControlNetExample {
    Invoke-GoExample -ExamplePath "./malina-controlnet/main.go"
}

# ./pwr.ps1 example-malina-upscale
function Invoke-MalinaUpscaleExample {
    Invoke-GoExample -ExamplePath "./malina-upscale/main.go"
}

# ./pwr.ps1 example-malina-adetailer
function Invoke-MalinaADetailerExample {
    Invoke-GoExample -ExamplePath "./malina-adetailer/main.go"
}

# ./pwr.ps1 example-malina-animatediff
function Invoke-MalinaAnimateDiffExample {
    Invoke-GoExample -ExamplePath "./malina-animatediff/main.go"
}

# ./pwr.ps1 example-malina-s2v -Arguments "--image", "portrait.png", "--audio", "speech.wav"
function Invoke-MalinaSpeechToVideoExample {
    param([string[]]$ExampleArguments = @())

    Invoke-GoExample -ExamplePath "./malina-s2v/main.go" -AdditionalArguments $ExampleArguments
}

# ./pwr.ps1 example-pool
function Invoke-PoolExample {
    Invoke-GoExample -ExamplePath "./pool/main.go"
}

# ./pwr.ps1 example-rag
function Invoke-RagExample {
    Invoke-GoExample -ExamplePath "./rag/main.go"
}

# ./pwr.ps1 example-rerank
function Invoke-RerankExample {
    Invoke-GoExample -ExamplePath "./rerank/main.go"
}

# ./pwr.ps1 example-question
function Invoke-QuestionExample {
    Invoke-GoExample -ExamplePath "./question/main.go"
}

# ./pwr.ps1 example-response
function Invoke-ResponseExample {
    Invoke-GoExample -ExamplePath "./response/main.go"
}

# ./pwr.ps1 example-session-store
function Invoke-SessionStoreExample {
    Invoke-GoExample -ExamplePath "./session-store/..."
}

# ./pwr.ps1 example-vision
function Invoke-VisionExample {
    Invoke-GoExample -ExamplePath "./vision/main.go"
}

# ------------------------------------------------------------------------------
# Yzma

# ./pwr.ps1 example-yzma-step1
function Invoke-YzmaStep1Example {
    Invoke-GoExample -ExamplePath "./yzma/step1/main.go"
}

# ./pwr.ps1 example-yzma-step2
function Invoke-YzmaStep2Example {
    Invoke-GoExample -ExamplePath "./yzma/step2/main.go"
}

# ./pwr.ps1 example-yzma-step3
function Invoke-YzmaStep3Example {
    Invoke-GoExample -ExamplePath "./yzma/step3/main.go"
}

# ./pwr.ps1 example-yzma-step4
function Invoke-YzmaStep4Example {
    Invoke-GoExample -ExamplePath "./yzma/step4/main.go"
}

# ./pwr.ps1 example-yzma-step5
function Invoke-YzmaStep5Example {
    Invoke-GoExample -ExamplePath "./yzma/step5/main.go"
}

# ./pwr.ps1 example-yzma-step6
function Invoke-YzmaStep6Example {
    Invoke-GoExample -ExamplePath "./yzma/step6/main.go"
}

# ./pwr.ps1 example-yzma-step7
function Invoke-YzmaStep7Example {
    Invoke-GoExample -ExamplePath "./yzma/step7/main.go"
}

# ./pwr.ps1 example-yzma-step8
function Invoke-YzmaStep8Example {
    Invoke-GoExample -ExamplePath "./yzma/step8/main.go"
}

function Invoke-YzmaCurlRequest {
    param(
        [Parameter(Mandatory)]
        [string[]]$CurlArguments,

        [string]$Body,

        [switch]$NoBody
    )

    if ($NoBody) {
        & curl.exe @CurlArguments
    }
    else {
        $Body | & curl.exe @CurlArguments --data-binary "@-"
    }
    Assert-NativeCommandSucceeded -Command "curl.exe"
}

# ./pwr.ps1 example-yzma-parallel-curl1
function Invoke-YzmaParallelCurl1 {
    $body = '{"prompt":"Hello, how are you?","max_tokens":50}'
    Invoke-YzmaCurlRequest `
        -CurlArguments @(
            "--request", "POST",
            "http://localhost:8090/v1/completions",
            "--header", "Content-Type: application/json"
        ) `
        -Body $body
}

# ./pwr.ps1 example-yzma-parallel-curl2
function Invoke-YzmaParallelCurl2 {
    $body = '{"prompt":"Hello","max_tokens":50,"stream":true}'
    Invoke-YzmaCurlRequest `
        -CurlArguments @(
            "--request", "POST",
            "http://localhost:8090/v1/completions",
            "--header", "Content-Type: application/json"
        ) `
        -Body $body
}

# ./pwr.ps1 example-yzma-parallel-curl3
function Invoke-YzmaParallelCurl3 {
    Invoke-YzmaCurlRequest `
        -CurlArguments @("http://localhost:8090/v1/stats") `
        -NoBody
}

# ./pwr.ps1 example-yzma-parallel-load
function Invoke-YzmaParallelLoad {
    $jobs = @(
        foreach ($requestNumber in 1..20) {
            $body = ConvertTo-Json `
                -Compress `
                -InputObject @{
                    prompt     = "Request ${requestNumber}: Hello"
                    max_tokens = 30
                }

            Start-Job -ArgumentList $body -ScriptBlock {
                param([string]$RequestBody)

                $RequestBody | & curl.exe `
                    --silent `
                    --request POST `
                    "http://localhost:8090/v1/completions" `
                    --header "Content-Type: application/json" `
                    --data-binary "@-"
                if ($LASTEXITCODE -ne 0) {
                    throw "curl.exe exited with code $LASTEXITCODE"
                }
            }
        }
    )

    try {
        Wait-Job -Job $jobs | Out-Null
        Receive-Job -Job $jobs

        $failedJobs = @($jobs | Where-Object { $_.State -ne "Completed" })
        if ($failedJobs.Count -gt 0) {
            throw "$($failedJobs.Count) parallel request(s) failed"
        }
    }
    finally {
        Remove-Job -Job $jobs -Force -ErrorAction SilentlyContinue
    }
}
