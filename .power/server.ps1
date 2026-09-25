# ==============================================================================
# Kronk BUI

# Install the Kronk BUI dependencies.
# ./pwr.ps1 bui-install
function Install-Bui {
    Invoke-InDirectory -Path $BuiDirectory -Action {
        npm install
        Assert-NativeCommandSucceeded -Command "npm"
    }
}

# Build the Kronk BUI.
# ./pwr.ps1 bui-build
function Build-Bui {
    Invoke-InDirectory -Path $BuiDirectory -Action {
        npm run build
        Assert-NativeCommandSucceeded -Command "npm"
    }
}

# ==============================================================================
# Kronk Server

# Generate the Kronk documentation.
# ./pwr.ps1 kronk-docs
function Build-KronkDocs {
    Invoke-InDirectory -Path $RepoRoot -Action {
        go run ./cmd/server/api/tooling/docs
        Assert-NativeCommandSucceeded -Command "go"
    }
}

# Generate the Kronk documentation and build the BUI.
# ./pwr.ps1 kronk-build
function Build-Kronk {
    Build-KronkDocs
    Build-Bui
}
