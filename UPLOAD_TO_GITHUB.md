# Загрузка в GitHub без Codex

1. Открой `https://github.com/trollek31/VideoTrimmer`.
2. Нажми `Add file` → `Upload files`.
3. Распакуй этот архив и перетащи **всё содержимое папки проекта** в окно GitHub. Важно сохранить `.github/workflows/windows-release.yml`.
4. Нажми `Commit changes` в ветку `main`.
5. Открой вкладку `Actions`. Workflow `Build Windows portable release` должен запуститься автоматически.
6. После успешной проверки открой `Releases` — там появится `VideoTrimmer_Windows.zip`.

GitHub Actions собирает Windows x64 на `windows-latest`, скачивает нужный FFmpeg runtime и проверяет:
- наличие ffmpeg/ffprobe/ffplay;
- генерацию тестового MP4;
- trim без перекодирования;
- точный trim с перекодированием;
- корректную длительность результата;
- запуск `VideoTrimmer.exe`.
