//go:build windows

package vision

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

type ocrResult struct {
	Text  string     `json:"text"`
	Lines []TextLine `json:"lines"`
}

type winOCRPayload struct {
	OK     bool    `json:"ok"`
	Error  string  `json:"error,omitempty"`
	Text   string  `json:"text"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Lines  []struct {
		Text   string  `json:"text"`
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
		Words  []struct {
			Text   string  `json:"text"`
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"words"`
	} `json:"lines"`
}

const createNoWindow = 0x08000000

// One hidden PowerShell/WinRT worker is kept alive for the lifetime of JACoB.
// OCR requests are serialized through it. This avoids flashing a console window
// and avoids paying PowerShell + WinRT startup cost for every text operation.
type ocrWorker struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr bytes.Buffer
}

var sharedOCRWorker ocrWorker

func ocrText(img image.Image) (string, error) {
	r, err := ocrTextLines(img)
	if err != nil {
		return "", err
	}
	return r.Text, nil
}

func ocrTextLines(img image.Image) (ocrResult, error) {
	// The cropped/preprocessed Elite region remains inside the core. The worker
	// receives only a random private temporary path; the PNG is removed as soon
	// as recognition completes. No image bytes are returned to the browser SDK.
	f, err := os.CreateTemp("", "jacob-vision-ocr-*.png")
	if err != nil {
		return ocrResult{}, err
	}
	path := f.Name()
	defer os.Remove(path)
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return ocrResult{}, err
	}
	if err := f.Close(); err != nil {
		return ocrResult{}, err
	}

	raw, err := sharedOCRWorker.recognize(path)
	if err != nil {
		return ocrResult{}, err
	}
	var payload winOCRPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ocrResult{}, fmt.Errorf("Windows OCR returned invalid structured output: %w", err)
	}
	if !payload.OK {
		message := strings.TrimSpace(payload.Error)
		if message == "" {
			message = "unknown Windows OCR worker error"
		}
		return ocrResult{}, fmt.Errorf("Windows OCR failed: %s", message)
	}
	out := ocrResult{Text: strings.TrimSpace(payload.Text), Lines: make([]TextLine, 0, len(payload.Lines))}
	if payload.Width <= 0 || payload.Height <= 0 {
		return out, nil
	}
	for _, line := range payload.Lines {
		text := strings.TrimSpace(line.Text)
		if text == "" {
			continue
		}
		words := make([]TextWord, 0, len(line.Words))
		for _, word := range line.Words {
			wordText := strings.TrimSpace(word.Text)
			if wordText == "" {
				continue
			}
			words = append(words, TextWord{Text: wordText, Bounds: Region{
				X: word.X / payload.Width, Y: word.Y / payload.Height,
				Width: word.Width / payload.Width, Height: word.Height / payload.Height,
			}})
		}
		out.Lines = append(out.Lines, TextLine{Text: text, Bounds: Region{
			X: line.X / payload.Width, Y: line.Y / payload.Height,
			Width: line.Width / payload.Width, Height: line.Height / payload.Height,
		}, Words: words})
	}
	return out, nil
}

func (w *ocrWorker) recognize(path string) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if w.cmd == nil {
			if err := w.start(); err != nil {
				lastErr = err
				continue
			}
		}
		if _, err := io.WriteString(w.stdin, path+"\n"); err != nil {
			lastErr = fmt.Errorf("Windows OCR worker input failed: %w", err)
			w.stop()
			continue
		}
		line, err := w.stdout.ReadBytes('\n')
		if err != nil {
			message := strings.TrimSpace(w.stderr.String())
			if message != "" {
				lastErr = fmt.Errorf("Windows OCR worker stopped: %s", message)
			} else {
				lastErr = fmt.Errorf("Windows OCR worker stopped: %w", err)
			}
			w.stop()
			continue
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			lastErr = fmt.Errorf("Windows OCR worker returned an empty response")
			w.stop()
			continue
		}
		return append([]byte(nil), line...), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Windows OCR worker unavailable")
	}
	return nil, lastErr
}

func newOCRWorkerCommand() *exec.Cmd {
	cmd := exec.Command(
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass",
		"-Command", ocrWorkerScript,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

func (w *ocrWorker) start() error {
	w.stop()
	w.stderr.Reset()
	cmd := newOCRWorkerCommand()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("Windows OCR worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("Windows OCR worker stdout: %w", err)
	}
	cmd.Stderr = &w.stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("Windows OCR worker start failed: %w", err)
	}
	w.cmd = cmd
	w.stdin = stdin
	w.stdout = bufio.NewReaderSize(stdout, 128*1024)
	return nil
}

func (w *ocrWorker) stop() {
	if w.stdin != nil {
		_ = w.stdin.Close()
	}
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
		_ = w.cmd.Wait()
	}
	w.cmd = nil
	w.stdin = nil
	w.stdout = nil
}

const ocrWorkerScript = `$ErrorActionPreference='Stop'
$utf8=New-Object System.Text.UTF8Encoding($false)
[Console]::OutputEncoding=$utf8
[Console]::InputEncoding=$utf8
Add-Type -AssemblyName System.Runtime.WindowsRuntime
[Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime] > $null
[Windows.Storage.FileAccessMode,Windows.Storage,ContentType=WindowsRuntime] > $null
[Windows.Storage.Streams.IRandomAccessStream,Windows.Storage.Streams,ContentType=WindowsRuntime] > $null
[Windows.Graphics.Imaging.BitmapDecoder,Windows.Graphics.Imaging,ContentType=WindowsRuntime] > $null
[Windows.Graphics.Imaging.SoftwareBitmap,Windows.Graphics.Imaging,ContentType=WindowsRuntime] > $null
[Windows.Media.Ocr.OcrEngine,Windows.Foundation,ContentType=WindowsRuntime] > $null
$asTaskGeneric=([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object { $_.Name -eq 'AsTask' -and $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name.StartsWith('IAsyncOperation') } | Select-Object -First 1)
function Await($op,[Type]$type){$t=$asTaskGeneric.MakeGenericMethod($type).Invoke($null,@($op));$t.Wait();return $t.Result}
$engine=[Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
if($null -eq $engine){throw 'No Windows OCR language is available for the current language profile'}
while($true){
  $path=[Console]::In.ReadLine()
  if($null -eq $path){break}
  try {
    $file=Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($path)) ([Windows.Storage.StorageFile])
    $stream=Await ($file.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
    try {
      $decoder=Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
      $bitmap=Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
      $result=Await ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
      $lines=@()
      foreach($line in $result.Lines){
        $minX=[double]::PositiveInfinity; $minY=[double]::PositiveInfinity
        $maxX=[double]::NegativeInfinity; $maxY=[double]::NegativeInfinity
        foreach($word in $line.Words){
          $r=$word.BoundingRect
          if($r.X -lt $minX){$minX=$r.X}; if($r.Y -lt $minY){$minY=$r.Y}
          if(($r.X+$r.Width) -gt $maxX){$maxX=$r.X+$r.Width}
          if(($r.Y+$r.Height) -gt $maxY){$maxY=$r.Y+$r.Height}
        }
        if([double]::IsInfinity($minX)){$minX=0;$minY=0;$maxX=0;$maxY=0}
        $words=@()
        foreach($word in $line.Words){
          $wr=$word.BoundingRect
          $words += [pscustomobject]@{text=$word.Text;x=$wr.X;y=$wr.Y;width=$wr.Width;height=$wr.Height}
        }
        $lines += [pscustomobject]@{text=$line.Text;x=$minX;y=$minY;width=($maxX-$minX);height=($maxY-$minY);words=$words}
      }
      $payload=[pscustomobject]@{ok=$true;text=$result.Text;width=$bitmap.PixelWidth;height=$bitmap.PixelHeight;lines=$lines}
    } finally {
      if($null -ne $stream){$stream.Dispose()}
    }
  } catch {
    $payload=[pscustomobject]@{ok=$false;error=$_.Exception.Message;text='';width=0;height=0;lines=@()}
  }
  [Console]::Out.WriteLine(($payload | ConvertTo-Json -Compress -Depth 5))
  [Console]::Out.Flush()
}`
