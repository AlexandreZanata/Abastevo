package com.anpfuel.data.parser

/**
 * Excel cell coordinates with range validation (SEC06-B): references
 * resolve before any allocation, so a crafted `ZZZZZZ1` refuses
 * instead of driving a 300M-entry dense row (AUD-10). Excel bounds
 * are hard maxima (XFD = 16384 columns, 1048576 rows); anything
 * outside refuses closed.
 */
internal object XlsxCellReference {

    /** Maximum column letters: XFD is the widest valid reference. */
    private const val MAX_COLUMN_LETTERS = 3

    /** Maximum zero-based column index (XFD). */
    const val MAX_COLUMN_INDEX = 16383

    /** Maximum one-based row number. */
    const val MAX_ROW_NUMBER = 1_048_576

    fun columnIndex(cellReference: String): Int {
        val letters = cellReference.takeWhile { it.isLetter() }
        require(letters.isNotEmpty() && letters.length <= MAX_COLUMN_LETTERS &&
            letters.all { it in 'A'..'Z' || it in 'a'..'z' }
        ) {
            "Cell reference is outside the Excel grid: $cellReference"
        }
        var index = 0
        for (character in letters.uppercase()) {
            index = index * 26 + (character.code - 'A'.code + 1)
        }
        require(index in 1..MAX_COLUMN_INDEX + 1) {
            "Cell reference is outside the Excel grid: $cellReference"
        }
        return index - 1
    }

    fun rowNumber(cellReference: String): Int {
        val digits = cellReference.dropWhile { it.isLetter() }
        val row = digits.toIntOrNull()
        require(!digits.isEmpty() && row != null && row in 1..MAX_ROW_NUMBER) {
            "Cell reference is outside the Excel grid: $cellReference"
        }
        return row
    }
}
