package com.anpfuel.data.parser

internal object XlsxCellReference {

    fun columnIndex(cellReference: String): Int {
        val letters = cellReference.takeWhile { it.isLetter() }
        var index = 0
        for (character in letters.uppercase()) {
            index = index * 26 + (character.code - 'A'.code + 1)
        }
        return index - 1
    }

    fun rowNumber(cellReference: String): Int =
        cellReference.dropWhile { it.isLetter() }.toInt()
}
