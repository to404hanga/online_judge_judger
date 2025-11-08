package config

import ojmodel "github.com/to404hanga/online_judge_common/model"

// LanguageConfig defines the commands and settings for a specific language.
type LanguageConfig struct {
	ImageName      string
	SourceFileName string
	BuildCommand   []string
	RunCommand     []string
}

var LanguageConfigs = map[ojmodel.SubmissionLanguage]LanguageConfig{
	ojmodel.SubmissionLanguageCPP: {
		ImageName:      "judge-cpp:latest",
		SourceFileName: "main.cpp",
		BuildCommand:   []string{"g++", "-std=c++17", "-o", "/app/main", "/app/main.cpp"},
		RunCommand:     []string{"/app/main"},
	},
	ojmodel.SubmissionLanguageC: {
		ImageName:      "judge-c:latest",
		SourceFileName: "main.c",
		BuildCommand:   []string{"gcc", "-std=c17", "-o", "/app/main", "/app/main.c"},
		RunCommand:     []string{"/app/main"},
	},
	ojmodel.SubmissionLanguageJava: {
		ImageName:      "judge-java:latest",
		SourceFileName: "Main.java",
		BuildCommand:   []string{"javac", "-d", "/app", "/app/Main.java"},
		RunCommand:     []string{"java", "-cp", "/app", "Main"},
	},
	ojmodel.SubmissionLanguagePython: {
		ImageName:      "judge-python:latest",
		SourceFileName: "main.py",
		BuildCommand:   nil, // Interpreted language
		RunCommand:     []string{"python3", "/app/main.py"},
	},
	ojmodel.SubmissionLanguageGo: {
		ImageName:      "judge-go:latest",
		SourceFileName: "main.go",
		BuildCommand:   []string{"go", "build", "-o", "/app/main", "/app/main.go"},
		RunCommand:     []string{"/app/main"},
	},
}
