// Wine Game Wrapper - Frontend Logic

(function () {
    "use strict";

    // DOM elements
    const gameSelect = document.getElementById("game-select");
    const customExeGroup = document.getElementById("custom-exe-group");
    const customExeInput = document.getElementById("custom-exe");
    const cuePath = document.getElementById("cue-path");
    const cueGroup = document.getElementById("cue-group");
    const srcGroup = document.getElementById("src-group");
    const srcPath = document.getElementById("src-path");
    const btnBrowseSrc = document.getElementById("btn-browse-src");
    const outputPath = document.getElementById("output-path");
    const winePath = document.getElementById("wine-path");
    const otvdmPath = document.getElementById("otvdm-path");
    const mcicdaPath = document.getElementById("mcicda-path");
    const win16Check = document.getElementById("win16-check");
    const btnBrowseCue = document.getElementById("btn-browse-cue");
    const btnBrowseOutput = document.getElementById("btn-browse-output");
    const btnBrowseWine = document.getElementById("btn-browse-wine");
    const btnBrowseOtvdm = document.getElementById("btn-browse-otvdm");
    const btnBrowseMcicda = document.getElementById("btn-browse-mcicda");
    const btnBuild = document.getElementById("btn-build");
    const btnClearLog = document.getElementById("btn-clear-log");
    const progressSection = document.getElementById("progress-section");
    const progressBar = document.getElementById("progress-bar");
    const progressText = document.getElementById("progress-text");
    const stepName = document.getElementById("step-name");
    const logArea = document.getElementById("log-area");

    let profiles = [];
    let initDone = false;

    // Initialize
    async function init() {
        if (initDone) return;
        initDone = true;

        try {
            profiles = await window['go']['main']['App']['GetProfiles']();
            populateProfiles();
            appendLog("Ready. Select a game and CUE file to begin.", "step");
        } catch (e) {
            appendLog("Failed to load profiles: " + e, "error");
        }

        setupEventListeners();
        setupWailsEvents();
    }

    function populateProfiles() {
        profiles.forEach(function (p) {
            var opt = document.createElement("option");
            opt.value = p.slug;
            opt.textContent = p.name + (p.win16 ? " (Win16)" : "");
            gameSelect.appendChild(opt);
        });

        // Add custom option
        var customOpt = document.createElement("option");
        customOpt.value = "custom";
        customOpt.textContent = "Custom (specify exe)";
        gameSelect.appendChild(customOpt);
    }

    function setupEventListeners() {
        // Game selection
        gameSelect.addEventListener("change", function () {
            var isCustom = gameSelect.value === "custom";
            customExeGroup.style.display = isCustom ? "flex" : "none";

            // Auto-set win16 checkbox for known profiles
            var profile = profiles.find(function (p) { return p.slug === gameSelect.value; });
            if (profile) {
                win16Check.checked = profile.win16;
            }

            // Folder-sourced games (e.g. a GOG install) are built from a
            // directory, not a CUE — swap the picker accordingly.
            var folderSource = profile && profile.folderSource;
            cueGroup.style.display = folderSource ? "none" : "block";
            srcGroup.style.display = folderSource ? "block" : "none";

            updateBuildButton();
        });

        customExeInput.addEventListener("input", updateBuildButton);

        // File browsers
        btnBrowseCue.addEventListener("click", async function () {
            try {
                var path = await window['go']['main']['App']['SelectCUEFile']();
                if (path) {
                    cuePath.value = path;
                    appendLog("Selected CUE: " + path);
                    updateBuildButton();
                }
            } catch (e) {
                appendLog("File dialog error: " + e, "error");
            }
        });

        btnBrowseSrc.addEventListener("click", async function () {
            try {
                var path = await window['go']['main']['App']['SelectDirectory']("Select Game Folder");
                if (path) {
                    srcPath.value = path;
                    appendLog("Selected folder: " + path);
                    updateBuildButton();
                }
            } catch (e) {
                appendLog("File dialog error: " + e, "error");
            }
        });

        btnBrowseOutput.addEventListener("click", async function () {
            try {
                var gameName = "";
                var sel = gameSelect.value;
                if (sel && sel !== "custom") {
                    for (var i = 0; i < profiles.length; i++) {
                        if (profiles[i].slug === sel) { gameName = profiles[i].name; break; }
                    }
                }
                var path = await window['go']['main']['App']['SelectOutputPath'](gameName);
                if (path) {
                    outputPath.value = path;
                }
            } catch (e) {
                appendLog("File dialog error: " + e, "error");
            }
        });

        btnBrowseWine.addEventListener("click", async function () {
            try {
                var path = await window['go']['main']['App']['SelectDirectory']("Select Wine Installation Directory");
                if (path) winePath.value = path;
            } catch (e) {
                appendLog("File dialog error: " + e, "error");
            }
        });

        btnBrowseOtvdm.addEventListener("click", async function () {
            try {
                var path = await window['go']['main']['App']['SelectDirectory']("Select OTVDM Build Directory");
                if (path) otvdmPath.value = path;
            } catch (e) {
                appendLog("File dialog error: " + e, "error");
            }
        });

        btnBrowseMcicda.addEventListener("click", async function () {
            try {
                var path = await window['go']['main']['App']['SelectFile']("Select mcicda.dll");
                if (path) mcicdaPath.value = path;
            } catch (e) {
                appendLog("File dialog error: " + e, "error");
            }
        });

        // Build button
        btnBuild.addEventListener("click", startBuild);

        // Clear log
        btnClearLog.addEventListener("click", function () {
            logArea.innerHTML = "";
        });
    }

    function setupWailsEvents() {
        window.runtime.EventsOn("build:log", function (msg) {
            // Detect step lines
            if (/^\[\d+\/\d+\]/.test(msg)) {
                appendLog(msg, "step");
            } else if (/Build complete/.test(msg)) {
                appendLog(msg, "success");
            } else {
                appendLog(msg);
            }
        });

        window.runtime.EventsOn("build:step", function (data) {
            progressSection.style.display = "block";
            var pct = Math.round((data.step / data.total) * 100);
            progressBar.style.width = pct + "%";
            progressText.textContent = data.step + "/" + data.total;
            stepName.textContent = data.name;
        });

        window.runtime.EventsOn("build:error", function (msg) {
            appendLog("Error: " + msg, "error");
        });

        window.runtime.EventsOn("build:complete", function (data) {
            if (data.success) {
                btnBuild.className = "btn-build success";
                btnBuild.querySelector(".btn-text").textContent = "Build Complete!";
                progressBar.style.width = "100%";
                progressText.textContent = "7/7";
                stepName.textContent = "Done!";
            } else {
                btnBuild.className = "btn-build error";
                btnBuild.querySelector(".btn-text").textContent = "Build Failed";
                if (data.error) {
                    appendLog("Build failed: " + data.error, "error");
                }
            }

            // Re-enable after a short delay
            setTimeout(function () {
                btnBuild.disabled = false;
                btnBuild.className = "btn-build";
                btnBuild.querySelector(".btn-text").textContent = "Build .app";
                updateBuildButton();
            }, 3000);
        });
    }

    function currentProfile() {
        return profiles.find(function (p) { return p.slug === gameSelect.value; });
    }

    function updateBuildButton() {
        var gameOk = gameSelect.value && (gameSelect.value !== "custom" || customExeInput.value.trim());
        var profile = currentProfile();
        var inputOk = (profile && profile.folderSource)
            ? srcPath.value.trim() !== ""
            : cuePath.value.trim() !== "";
        btnBuild.disabled = !(gameOk && inputOk);
    }

    async function startBuild() {
        var config = {
            gameSlug: gameSelect.value === "custom" ? "" : gameSelect.value,
            customExe: customExeInput.value.trim(),
            cuePath: cuePath.value.trim(),
            sourceDir: srcPath.value.trim(),
            outputPath: outputPath.value.trim(),
            winePath: winePath.value.trim(),
            otvdmPath: otvdmPath.value.trim(),
            mcicdaPath: mcicdaPath.value.trim(),
            win16: win16Check.checked,
        };

        // Reset UI
        logArea.innerHTML = "";
        btnBuild.disabled = true;
        btnBuild.className = "btn-build building";
        btnBuild.querySelector(".btn-text").textContent = "Building...";
        progressSection.style.display = "block";
        progressBar.style.width = "0%";
        progressText.textContent = "0/7";
        stepName.textContent = "Starting...";

        appendLog("Starting build...", "step");
        appendLog("Game: " + (config.gameSlug || "Custom (" + config.customExe + ")"));
        if (config.sourceDir && (currentProfile() && currentProfile().folderSource)) {
            appendLog("Folder: " + config.sourceDir);
        } else {
            appendLog("CUE:  " + config.cuePath);
        }
        if (config.outputPath) {
            appendLog("Output: " + config.outputPath);
        }
        appendLog("");

        try {
            await window['go']['main']['App']['StartBuild'](config);
        } catch (e) {
            appendLog("Failed to start build: " + e, "error");
            btnBuild.disabled = false;
            btnBuild.className = "btn-build error";
            btnBuild.querySelector(".btn-text").textContent = "Build Failed";
        }
    }

    function appendLog(msg, type) {
        var line = document.createElement("div");
        line.textContent = msg;
        if (type === "step") {
            line.className = "log-step";
        } else if (type === "error") {
            line.className = "log-error";
        } else if (type === "success") {
            line.className = "log-success";
        }
        logArea.appendChild(line);
        logArea.scrollTop = logArea.scrollHeight;
    }

    // Wait for Wails to be ready
    document.addEventListener("DOMContentLoaded", function () {
        if (window.runtime && window['go']) {
            init();
        } else {
            // Poll for runtime - Wails injects it asynchronously
            var check = setInterval(function () {
                if (window.runtime && window['go']) {
                    clearInterval(check);
                    init();
                }
            }, 50);
        }
    });
})();
