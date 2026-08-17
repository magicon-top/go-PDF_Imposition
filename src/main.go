package main
//Never delete this comment! In the code, empty lines like //______ should only appear before functions, followed below by a function description in English (2–5 lines). All other comments must only be at the end of lines. No empty lines. Keep code compact. Do not delete or format anything on your own!!
import ( "bufio";   "fmt";  "os";   "path/filepath";    "regexp";   "runtime";  "strconv";  "strings";  "syscall" 
     
    "github.com/pdfcpu/pdfcpu/pkg/api"
    "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
    "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
    "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

//  "github.com/magicon-top/go-pkg/pdfcpu/pkg/api"
//  "github.com/magicon-top/go-pkg/pdfcpu/pkg/pdfcpu"
//  "github.com/magicon-top/go-pkg/pdfcpu/pkg/pdfcpu/model"
//  "github.com/magicon-top/go-pkg/pdfcpu/pkg/pdfcpu/types"
    "golang.org/x/term"
    "github.com/magicon-top/go-pkg/pdfparser"
)

const ( cReset     = "\033[0m";           cGreen     = "\033[32m";      cOrange   = "\033[38;5;208m";    cGreenBg  = "\033[42;97m";  cOrangeBg = "\033[48;5;208;97m"
)

var (
    reDigit       = regexp.MustCompile(`\d+`)
    reGroup       = regexp.MustCompile(`\(([^)]+)\)\*(\d+)`)
    reLayoutChars = regexp.MustCompile(`^[0-9()+\-* \t]+$`)
)

type StampItem struct {
    Page string
    Rot  int
} // Structural element: page number and rotation angle

// __________________________________________________
// Enables virtual terminal processing for ANSI color support in Windows console.
// Checks if running on Windows and sets the ENABLE_VIRTUAL_TERMINAL_PROCESSING flag.
func enableVirtualTerminal() { // Enable ANSI color support in Windows console
    if runtime.GOOS == "windows" { // Check if running under Windows OS
        handle := syscall.Handle(os.Stdout.Fd()) // Get standard output handle
        var mode uint32                          // Variable for current console mode
        if err := syscall.GetConsoleMode(handle, &mode); err == nil { // Request current console mode
            syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode").Call(uintptr(handle), uintptr(mode|0x0004)) // Enable ENABLE_VIRTUAL_TERMINAL_PROCESSING flag
        }
    }
}

// __________________________________________________
// Cleans up temporary pdfcpu files left in the system Temp folder.
// Searches for files matching the pdfcpu pattern and removes them.
func cleanupTempFiles() { // Clean up temporary files left by pdfcpu in system Temp folder
    tempDir := os.TempDir()                       // Get system Temp directory path
    pattern := filepath.Join(tempDir, "pdfcpu_*") // Search pattern for temp files
    files, err := filepath.Glob(pattern)          // Search files matching pattern
    if err != nil {
        return
    } // Exit on search error
    for _, f := range files {
        _ = os.Remove(f)
    } // Delete found temporary files
}

// __________________________________________________
// Validates the syntax of the configuration file 0.txt.
// Checks layout format, line structures, key-value syntax, and bracket balance.
func validateConfigFile(filePath string) error { // Validate configuration 0.txt syntax
    file, err := os.Open(filePath) // Open configuration file
    if err != nil {
        return fmt.Errorf("error opening file: %v", err)
    } // Return error if file is missing
    defer file.Close() // Ensure file is closed on exit
    scanner, lineNum := bufio.NewScanner(file), 0 // Initialize scanner and line counter
    for scanner.Scan() { // Read file line by line
        lineNum++
        line := strings.TrimSpace(scanner.Text()) // Increment line counter and trim spaces
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        } // Skip empty lines and comments
        if strings.Contains(line, "=") { // Check lines containing variables
            parts := strings.SplitN(line, "=", 2) // Split key and value
            if strings.TrimSpace(parts[0]) == "" {
                return fmt.Errorf("line %d: invalid variable format (empty key): %s", lineNum, line)
            } // Key format error
            continue // Proceed to next line, any characters after '=' are allowed
        }

        if !reLayoutChars.MatchString(line) {
            return fmt.Errorf("line %d contains invalid characters (letters, dots, etc.): %s", lineNum, line)
        } // Validate layout line syntax
        openBrackets := 0            // Bracket balance counter
        for i, char := range line { // Parse line character by character
            if char == '(' { // Handle opening bracket
                if openBrackets++; openBrackets > 1 {
                    return fmt.Errorf("line %d: nested brackets detected", lineNum)
                } // Prevent nested brackets
            } else if char == ')' { // Handle closing bracket
                if openBrackets--; openBrackets < 0 {
                    return fmt.Errorf("line %d: closing bracket ')' comes before opening '('", lineNum)
                } // Bracket order error
                if rest := strings.TrimSpace(line[i+1:]); !strings.HasPrefix(rest, "*") {
                    return fmt.Errorf("line %d: expected '*N' syntax after closing bracket ')'", lineNum)
                } // Check for multiplier syntax
            }
        }
        if openBrackets != 0 {
            return fmt.Errorf("line %d: unclosed bracket '('", lineNum)
        } // Unbalanced brackets error
    }
    return scanner.Err() // Return possible scanning errors
}

// __________________________________________________
// Main entry point for processing PDF files and creating imposition layouts.
// Reads parameters, calculates coordinates, parses target PDFs, and applies watermarks.
func main() { // Main application entry point
    enableVirtualTerminal() // Activate ANSI color mode for console
    workDir := "."          // Default working directory
    if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
        workDir = filepath.Clean(os.Args[1])
    } // Read working directory from arguments
    configPath := filepath.Join(workDir, "0.txt") // Build path to 0.txt file
    if err := validateConfigFile(configPath); err != nil {
        fmt.Printf("Syntax error in %s: %v\nProcessing stopped.\n", configPath, err)
        waitExit()
        return
    } // Stop on syntax error
    originalDir, _ := os.Getwd() // Store current working directory
    defer os.Chdir(originalDir)  // Restore initial directory on exit
    if err := os.Chdir(workDir); err != nil {
        fmt.Printf("Error switching to directory %s: %v\n", workDir, err)
        waitExit()
        return
    } // Switch to working directory
    zeroPdfBytes, err := os.ReadFile("0.pdf") // Read 0.pdf into memory once
    if err != nil {
        fmt.Printf("Error reading 0.pdf: %v\n", err)
        waitExit()
        return
    } // Exit on 0.pdf read error
    x, y, w, h, numXcalc, numYcalc, gapCalc, found, err := pdfparser.FindLowestLeftQurve3(zeroPdfBytes, 1, "*", "0-0-0-0-o") // Search for bottom-left object
    if err != nil {
        fmt.Printf("Error searching for die line in 0.pdf: %v\n", err)
        waitExit()
        return
    } // Exit on PDF parsing error
    if !found {
        fmt.Println("Error: Bottom-left die line object not found in 0.pdf (page 1)")
        waitExit()
        return
    } // Exit if object not found
    baseX, baseY, width, height := x, y, w, h // Save base metrics
    config := make(map[string]float64)        // Configuration parameters map
    configStr := make(map[string]string)      // String configuration parameters map
    var pages [][][]StampItem                 // 3D slice for pages, rows, and elements
    
    type RawBlock []string        // Type for raw block of lines
    var rawBlocks []RawBlock      // List of raw blocks

    err = func() error { // Read and parse 0.txt in isolated scope to release file handle immediately
        file, err := os.Open("0.txt") // Reopen 0.txt for parsing
        if err != nil {
            return err
        } // Return open error
        defer file.Close() // Close 0.txt file on exit from closure

        scanner := bufio.NewScanner(file) // Initialize configuration scanner
        var currentRawBlock RawBlock      // Current forming raw block

        for scanner.Scan() { // Read configuration line by line
            line := strings.TrimSpace(scanner.Text()) // Clean current line
            if line == "" || strings.HasPrefix(line, "#") { // Check for empty lines/comments
                if len(currentRawBlock) > 0 {
                    rawBlocks = append(rawBlocks, currentRawBlock)
                    currentRawBlock = nil
                } // Finalize current block
                continue // Proceed to next line
            }

            if strings.Contains(line, "=") { // Parse parameters with assignment
                parts := strings.SplitN(line, "=", 2) // Split key and value
                key := strings.TrimSpace(parts[0])
                valStr := parts[1] // Read everything after '=' without changes
                configStr[key] = valStr // Store full string value
                if val, parseErr := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(valStr), ",", "."), 64); parseErr == nil {
                    config[key] = val
                } // Store numeric value
            } else {
                currentRawBlock = append(currentRawBlock, line)
            } // Accumulate layout lines
        }
        if len(currentRawBlock) > 0 { rawBlocks = append(rawBlocks, currentRawBlock)
        } // Save last remaining block
        return scanner.Err()
    }()
    if err != nil {
        fmt.Printf("Error reading 0.txt: %v\n", err)
        waitExit()
        return
    } // Exit on read error

    numX, numY := numXcalc, numYcalc // Read layout grid
    if numX <= 0 {      numX = 4    } // Default numX value
    if numY <= 0 {      numY = 4    } // Default numY value
    maxItemsPerPage := numX * numY // Single print sheet capacity
    for _, blockLines := range rawBlocks { // Parse each raw block
        var blockItems []StampItem // List of block items
        for _, line := range blockLines { // Parse lines inside block
            line = reGroup.ReplaceAllStringFunc(line, func(match string) string { // Expand group multipliers
                submatches := reGroup.FindStringSubmatch(match)                    // Extract contents and multiplier
                inside, multiplier := strings.TrimSpace(submatches[1]), submatches[2] // Separate inner elements
                var expanded []string                                                 // Expanded strings list
                for _, item := range strings.Fields(inside) {
                    expanded = append(expanded, fmt.Sprintf("%s*%s", item, multiplier))
                } // Apply multiplier to elements
                return strings.Join(expanded, " ") // Return expanded substring
            })
            var fields []string
            for _, field := range strings.Fields(line) { // Parse single multipliers
                if strings.Contains(field, "*") { // Check individual multiplier
                    parts := strings.SplitN(field, "*", 2) // Separate item and count
                    if count, err := strconv.Atoi(parts[1]); err == nil && count > 0 { // Validate count
                        for i := 0; i < count; i++ {
                            fields = append(fields, parts[0])
                        } // Duplicate item
                        continue // Proceed to next field
                    }
                }
                fields = append(fields, field) // Add regular field
            }
            for _, field := range fields { // Parse items and rotation angles
                if num := reDigit.FindString(field); num != "" { // Extract page number
                    rot := (strings.Count(field, "-") * 90) - (strings.Count(field, "+") * 90) // Calculate rotation by symbols
                    rot = (rot % 360 + 360) % 360
                    if rot > 180 { rot -= 360 }
                    blockItems = append(blockItems, StampItem{Page: num, Rot: rot}) // Add stamp to list
                }
            }
        }
        if len(blockItems) > 0 { // If block contains items
            for p := 0; p < len(blockItems); p += maxItemsPerPage { // Slice into pages
                pageEnd := p + maxItemsPerPage // Calculate end index
                if pageEnd > len(blockItems) {
                    pageEnd = len(blockItems)
                } // Limit to list length
                pageChunk := blockItems[p:pageEnd] // Elements chunk for page
                var rows [][]StampItem             // Slice of page rows
                for i := 0; i < len(pageChunk); i += numX { // Slice into rows of numX
                    end := i + numX // Calculate row end
                    if end > len(pageChunk) {
                        end = len(pageChunk)
                    } // Limit to chunk length
                    rows = append(rows, pageChunk[i:end]) // Add row
                }
                pages = append(pages, rows) // Add page to layout
            }
        }
    }

    neededPages := len(pages) // Total required imposition pages
    if neededPages == 0 {   fmt.Printf("No page blocks found in 0.txt\n")
        waitExit();         return
    } // Exit if layout is empty
    entries, err := os.ReadDir(".") // Read directory to find target PDFs
    if err != nil { fmt.Printf("Error reading directory: %v\n", err)
        waitExit();         return
    } // Exit on directory read error
    var pdfFiles []string
    for _, entry := range entries { // Filter files
        name := entry.Name() // File name
        if !entry.IsDir() && strings.HasSuffix(strings.ToLower(name), ".pdf") && name != "0.pdf" && !strings.HasSuffix(name, "_SPUSK.pdf") { // Check extension and exceptions
            pdfFiles = append(pdfFiles, name) // Add file to list
        }
    }
    if len(pdfFiles) == 0 {         fmt.Printf("No matching PDF files found for processing\n")
        waitExit();         return
    } // Exit if no matching PDFs exist
    gap := gapCalc                           // Inter-column gap
    conf := model.NewDefaultConfiguration() // Configure pdfcpu
    conf.Unit, conf.Optimize = types.MILLIMETRES, false // Set millimeters and disable optimization

    var nameBase string // Base name string without sheet count
    var sheetCounts []string // Sheet counts per page
    var extraField string // Extra field for the 5th parameter
    if nameStr, ok := configStr["name"]; ok && nameStr != "" { // Read name variable from 0.txt
        parts := strings.Split(nameStr, "/") // Split by slash
        if len(parts) >= 4 { // Check if 4 fields are present
            nameBase = fmt.Sprintf("Заказ N%s / %s / Штамп %s", strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])) // Format first 3 fields
            for _, count := range strings.Split(parts[3], ",") { // Split 4th field by comma
                sheetCounts = append(sheetCounts, strings.TrimSpace(count)) // Add to counts slice
            }
            if len(parts) >= 5 { extraField = strings.TrimSpace(parts[4]) } // Extract 5th field if present
        } else {
            nameBase = nameStr // Fallback if format is not met
        }
    }

    inFiles := make([]string, neededPages)
    for i := range inFiles {
        inFiles[i] = "0.pdf"
    } // Reusable substrate array

    for _, pdfFileName := range pdfFiles { // Loop through each PDF file
        fmt.Printf("\n%s%s%s :\n", cGreen, pdfFileName, cReset) // Highlight current file name in green
        targetPdfBytes, err := os.ReadFile(pdfFileName)       // Read file into memory
        if err != nil {
            fmt.Printf("Error reading file %s into memory: %v\n", pdfFileName, err)
            continue
        } // Skip on read error
        bLeft, bRight, bTop, bBottom, err := pdfparser.GetPdfBleedsForPage(targetPdfBytes, 1) // Extract Bleed once from page 1
        targetPdfBytes = nil // Free memory reference for GC
        if err != nil { // Bleed detection error
            fmt.Printf("Error getting bleed for %s%s%s: %v\n", cGreen, pdfFileName, cReset, err) // Print Bleed error
            bLeft, bRight, bTop, bBottom = 0, 0, 0, 0                                             // Reset values to zero
        } else {
            fmt.Printf("Bleeds: L=%s%.2f%s, R=%s%.2f%s, T=%s%.2f%s, B=%s%.2f%s mm\n", cOrange, bLeft, cReset, cOrange, bRight, cReset, cOrange, bTop, cReset, cOrange, bBottom, cReset)
        } // Output Bleed info with orange highlighted values
        baseName := strings.TrimSuffix(pdfFileName, filepath.Ext(pdfFileName)) // File name without extension
        resultFile := baseName + "_SPUSK.pdf"                                 // Build result file name

        if err = api.MergeCreateFile(inFiles, resultFile, false, conf); err != nil {
            fmt.Printf("Error building base result PDF for %s: %v\n", pdfFileName, err)
            continue
        } // Create base imposition
        ctx, err := api.ReadContextFile(resultFile) // Load result into pdfcpu context
        if err != nil {     fmt.Printf("Error loading base result PDF into memory for %s: %v\n", resultFile, err)
            continue
        } // Skip on context read error

        for pageIdx, digitRows := range pages { // Iterate imposition pages
            targetPage, totalRows := pageIdx+1, len(digitRows) // Target page number and total rows
            fmt.Printf("\n%s      Page %d          %s\n", cGreenBg, targetPage, cReset) // Output log with orange page numbers and angles
            for rowIdx, row := range digitRows { // Iterate layout rows
                mid := len(row) / 2 // Middle of row for gap offset
                for colIdx, item := range row { // Iterate row columns
                    stampFile := fmt.Sprintf("%s:%s", pdfFileName, item.Page) // Reference to source sheet and page
                    fmt.Printf("%s%s%s %s%d%s\t", cOrange, item.Page, cReset, cReset, item.Rot, cReset) // Log page numbers and angles
                    calcX := float64(colIdx)*width + baseX // Calculate X position
                    if colIdx >= mid {
                        calcX += gap
                    } // Add gap after middle
                    calcY := float64(totalRows-1-rowIdx)*height + baseY - bBottom // Calculate Y position considering Bleed
                    calcX -= bLeft                                                // Adjust X position considering Bleed
                    offsetStr := fmt.Sprintf("pos:bl, off:%.3f %.3f, rot:%d, scale:1 abs", calcX, calcY, item.Rot) // Build watermark options
                    wm, err := pdfcpu.ParsePDFWatermarkDetails(stampFile, offsetStr, true, types.MILLIMETRES)        // Parse watermark details
                    if err != nil {
                        fmt.Printf("Error parsing stamp parameters for %s: %v\n", stampFile, err)
                        continue
                    } // Skip on parameters error
                    if err = pdfcpu.AddWatermarks(ctx, types.IntSet{targetPage: true}, wm); err != nil { // Apply stamp onto page
                        fmt.Printf("Error applying stamp %s on page %d: %v\n", stampFile, targetPage, err) // Log watermark error
                    }
                }
            }
            if nameBase != "" { // Apply parsed name stamp
                pageTextCMYK := nameBase // Initialize with base name
                if len(sheetCounts) > 0 { // Check if counts are available
                    countStr := sheetCounts[0] // Default to first count
                    if pageIdx < len(sheetCounts) { // If specific count exists for page
                        countStr = sheetCounts[pageIdx]
                    }
                    pageTextCMYK += fmt.Sprintf(" / %s листов чистыми", countStr) // Append sheet count
                }
                if extraField != "" { pageTextCMYK += " / " + extraField } // Append unchanged 5th field
                if cmykPdfBytes, cmykErr := pdfparser.GenerateCMYKTextPDFBytes("0.ttf", pageTextCMYK, 16, 70, 200, -90, "FFFFFFFF"); cmykErr == nil && len(cmykPdfBytes) > 0 { // Generate bytes
                    tempNameCmyk := fmt.Sprintf("temp_name_page_%d.pdf", targetPage) // Temp file name
                    if err := os.WriteFile(tempNameCmyk, cmykPdfBytes, 0644); err == nil { // Write to disk
                        cmykOffsetStr := "pos:bl, off:0 0, rot:0, scale:1 abs" // Offset parameters
                        if wmNameCmyk, err := pdfcpu.ParsePDFWatermarkDetails(tempNameCmyk, cmykOffsetStr, true, types.MILLIMETRES); err == nil { // Parse details
                            _ = pdfcpu.AddWatermarks(ctx, types.IntSet{targetPage: true}, wmNameCmyk) // Add watermark
                        }
                        _ = os.Remove(tempNameCmyk) // Clean up
                    }
                }
            }
            pageCmykText := fmt.Sprintf("Спуск %d /", targetPage) // Format per-page imposition CMYK text
            if pageCmykBytes, pageCmykErr := pdfparser.GenerateCMYKTextPDFBytes("0.ttf", pageCmykText, 16, 70, 175, -90, "FFFFFFFF"); pageCmykErr == nil && len(pageCmykBytes) > 0 { // Generate CMYK text PDF bytes for current page
                tempPageCmyk := fmt.Sprintf("temp_cmyk_page_%d.pdf", targetPage) // Temporary file name for per-page CMYK watermark
                if err := os.WriteFile(tempPageCmyk, pageCmykBytes, 0644); err == nil { // Write per-page CMYK PDF to disk
                    cmykOffsetStr := "pos:bl, off:0 0, rot:0, scale:1 abs" // Watermark positioning parameters
                    if wmPageCmyk, err := pdfcpu.ParsePDFWatermarkDetails(tempPageCmyk, cmykOffsetStr, true, types.MILLIMETRES); err == nil { // Parse per-page CMYK watermark details
                        _ = pdfcpu.AddWatermarks(ctx, types.IntSet{targetPage: true}, wmPageCmyk) // Apply per-page CMYK text watermark
                    }
                    _ = os.Remove(tempPageCmyk) // Clean up temporary per-page CMYK PDF file
                }
            }
        }

        if err = api.WriteContextFile(ctx, resultFile); err != nil {
            fmt.Printf("Error saving output PDF %s: %v\n", resultFile, err)
        } // Save generated file
        cleanupTempFiles() // Force cleanup of temporary pdfcpu .tmp files after processing each file
    }

    fmt.Printf("\n\n%s0.pdf%s: Bottom-left die corner: X = %s%.2f%s mm, Y = %s%.2f%s mm | Offsets between dies: X: %s%.2f%s mm, Y: %s%.2f%s mm\n", cGreen, cReset, cOrange, x, cReset, cOrange, y, cReset, cOrange, w, cReset, cOrange, h, cReset) // Print coordinates and sizes with color formatting
    fmt.Printf("Number of dies X-Y:  %s%d%s X %s%d%s | Central gutter gap: X: %s%.2f%s mm\n", cOrange, numXcalc, cReset, cOrange, numYcalc, cReset, cOrange, gapCalc, cReset)
    cleanupTempFiles() // Final Temp cleanup before exit
    fmt.Println("\nDone!")
    waitExit() // Display completion and pause
}

// __________________________________________________
// Pauses the console output and waits for key press before exiting.
// Uses terminal raw mode or fallback standard input reading across platforms.
func waitExit() { // Console pause function before closing
    fmt.Print("\nPress any key to exit...") // Output prompt to press key
    fd := int(os.Stdin.Fd())                 // Get stdin file descriptor
    if term.IsTerminal(fd) {                 // Check if process runs in terminal
        if oldState, err := term.MakeRaw(fd); err == nil {
            defer term.Restore(fd, oldState)
            var b [1]byte
            os.Stdin.Read(b[:])
            return
        } // Read single byte in raw mode
    }
    var b [1]byte // Fallback read buffer
    if runtime.GOOS == "windows" {
        os.Stdin.Read(b[:])
    } else {
        bufio.NewReader(os.Stdin).ReadBytes('\n')
    } // Cross-platform exit delay
}