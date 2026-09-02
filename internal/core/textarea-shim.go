package core

// **비계다.** `Buffer`·`Viewport` 를 `internal/textarea` 로 낸 뒤, core 가 쓰던 소문자 이름을
// 그대로 쓰게 해 주는 별칭이다.
//
// **여기 있는 줄 하나하나가 「core 가 그 패키지에 기대는 것」이다.** 컴파일러가 만들어 준
// 목록이고, 이 파일이 비면 가르기가 끝난 것이다 (ADR-0128).

import "github.com/bluemir/zn/internal/textarea"

const caseLower = textarea.CaseLower
const caseToggle = textarea.CaseToggle
const caseUpper = textarea.CaseUpper
const classBlank = textarea.ClassBlank
const defaultTabWidth = textarea.DefaultTabWidth
const gitLineAdded = textarea.GitLineAdded
const gitLineModified = textarea.GitLineModified
const gitLineRemoved = textarea.GitLineRemoved
const indentLeft = textarea.IndentLeft
const indentRight = textarea.IndentRight
const minTextWidth = textarea.MinTextWidth
const outsideCreated = textarea.OutsideCreated
const outsideModified = textarea.OutsideModified
const outsideRemoved = textarea.OutsideRemoved
const outsideSame = textarea.OutsideSame
const pageDown = textarea.PageDown
const pageFull = textarea.PageFull
const pageHalf = textarea.PageHalf
const pageUp = textarea.PageUp
const searchBackward = textarea.SearchBackward
const searchForward = textarea.SearchForward
const severityError = textarea.SeverityError
const severityWarning = textarea.SeverityWarning
const wordBig = textarea.WordBig
const wordSmall = textarea.WordSmall

type caseKind = textarea.CaseKind
type diagnostic = textarea.Diagnostic
type diagnosticSeverity = textarea.DiagnosticSeverity
type gitLineMark = textarea.GitLineMark
type indentDirection = textarea.IndentDirection
type outsideChange = textarea.OutsideChange
type pageDirection = textarea.PageDirection
type pageSpan = textarea.PageSpan
type screenRow = textarea.ScreenRow
type searchDirection = textarea.SearchDirection
type semanticToken = textarea.SemanticToken
type textBlock = textarea.TextBlock
type viewPlace = textarea.ViewPlace
type viewSize = textarea.ViewSize
type viewport = textarea.Viewport
type wordKind = textarea.WordKind

var controlText = textarea.ControlText
var errOpenFile = textarea.ErrOpenFile
var errWriteFile = textarea.ErrWriteFile
var glyphAt = textarea.GlyphAt
var glyphSize = textarea.GlyphSize
var newBuffer = textarea.NewBuffer
var newEmptyBuffer = textarea.NewEmptyBuffer
var offsetAtScreenCol = textarea.OffsetAtScreenCol
var openBuffer = textarea.OpenBuffer
var pageRows = textarea.PageRows
var prevGlyphStart = textarea.PrevGlyphStart
var replacementText = textarea.ReplacementText
var screenColAt = textarea.ScreenColAt
var screenText = textarea.ScreenText
var splitLines = textarea.SplitLines
var wrapOffsets = textarea.WrapOffsets

// 이미 대문자였던 것들. 이름은 그대로이므로 별칭으로 잇는다.
var OpenBuffer = textarea.OpenBuffer
var widthOf = textarea.WidthOf
var escapeSizeAt = textarea.EscapeSizeAt

type Buffer = textarea.Buffer

var countChangedLines = textarea.CountChangedLines
var detectReadOnly = textarea.DetectReadOnly

const markerWidth = textarea.MarkerWidth
const minAbsoluteDigits = textarea.MinAbsoluteDigits
const minRelativeDigits = textarea.MinRelativeDigits

var splitFormatted = textarea.SplitFormatted

const severityHint = textarea.SeverityHint

type selection = textarea.Selection

type viewTop = textarea.ViewTop
