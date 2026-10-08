package com.anpfuel.domain.portable

import com.anpfuel.domain.valueobject.FuelProduct
import kotlin.math.abs
import kotlin.math.max

/** Human-review suggestions from spatial OCR. No trust, publication or minimum-price selection. */
object FuelBoardOcr {
    data class Token(val text: String, val left: Int, val top: Int, val right: Int, val bottom: Int) {
        val height get() = max(1, bottom - top)
        val cx get() = (left + right) / 2.0
        val cy get() = (top + bottom) / 2.0
    }
    data class Row(val product: FuelProduct, val amountMilli: Long)
    data class Result(val rows: List<Row>, val conditional: Boolean, val unresolved: Boolean,
        val orphans: List<Long> = emptyList(), val conflictingProducts: Set<FuelProduct> = emptySet())
    private val price = Regex("(?<![0-9/.-])[0-9]{1,4}[.,][0-9]{2,3}(?![0-9/])")
    private val integerFragment = Regex("^[1-9][.,]?$")
    private val fractionFragment = Regex("^[0-9]{2,3}$")
    private val numericFragment = Regex("^[0-9]{1,3}[.,]?$")
    /** Board-only price recovery (NOT part of the frozen PortablePriceOcr grammar):
     * LED boards often lose the decimal separator ("4 35", "675").
     * Spaces between digits become a comma; a bare 3-digit token becomes
     * D,DD. Only whole-token values near a fuel label survive: matching
     * still requires anchor proximity and the review screen confirms. */
    private const val MAX_ORPHANS = 5

    /** Correlated pixel views improve suggestions, never confer trust or publication. */
    fun reconcile(views: List<Result>): Result {
        require(views.size in 1..3)
        val rows = mutableListOf<Row>()
        val manual = views.flatMap { it.orphans }.distinct().toMutableList()
        val conflicts = views.flatMap { it.conflictingProducts }.toMutableSet()
        var unresolved = views.any { it.unresolved }
        for (fuel in views.flatMap { it.rows }.map { it.product }.distinct()) {
            val votes = views.mapNotNull { view -> view.rows.singleOrNull { it.product == fuel }?.amountMilli }
                .groupingBy { it }.eachCount()
            val winner = votes.maxByOrNull { it.value } ?: continue
            if (fuel !in conflicts && (votes.size == 1 || winner.value >= 2)) rows += Row(fuel, winner.key)
            else {
                unresolved = true
                manual += votes.keys
            }
        }
        val ambiguous = rows.filter { row -> rows.any { other ->
            row.product != other.product && row.amountMilli == other.amountMilli &&
                views.none { view -> row in view.rows && other in view.rows }
        } }
        if (ambiguous.isNotEmpty()) {
            unresolved = true
            manual += ambiguous.map { it.amountMilli }
            conflicts += ambiguous.map { it.product }
            rows.removeAll(ambiguous.toSet())
        }
        return Result(rows, views.any { it.conditional }, unresolved,
            manual.distinct().take(MAX_ORPHANS), conflicts)
    }

    fun associate(input: List<Token>): Result {
        val original = input.take(512).filter { it.text.length <= 256 && it.right > it.left && it.bottom > it.top }.distinct()
        val recovered = recoverSplitPrices(original)
        // A three-digit fraction must not also become a standalone D,DD price.
        val tokens = original.filterNot { token ->
            recovered.any { joined -> joined.left <= token.left && joined.top <= token.top &&
                joined.right >= token.right && joined.bottom >= token.bottom } &&
                numericFragment.matches(token.text.trim())
        } + recovered + recoverBrandHeadings(original)
        val conditional = tokens.any { normalize(it.text).let { text -> listOf("VISTA", "CARTAO", "CREDITO", "DEBITO", "PIX", "APP", "CLUBE", "FIDELIDADE", "PROMOCAO").any { word -> Regex("\\b$word\\b").containsMatchIn(text) } } }
        fun encloses(a: Token,b: Token) = a.left <= b.left+2 && a.top <= b.top+2 && a.right >= b.right-2 && a.bottom >= b.bottom-2
        // Prefer tight element geometry over a whole line that contains both label and number.
        val labelTokens = tokens.filter { token -> labelLike(normalize(token.text)) }
            .filterNot { token -> tokens.any { child -> child != token && labelLike(normalize(child.text)) && encloses(token,child) && child.text.length < token.text.length && price.containsMatchIn(token.text) } }
        val anchors = labelTokens.map { token ->
            var text=normalize(token.text)
            var box=token
            if (!text.contains("ADITIV") && !text.contains("S10") && !text.contains("S500")) {
                val suffix = labelTokens.filter { other ->
                    other!=token && other.top>=token.top+token.height*0.35 && other.top-token.bottom <= token.height*0.65 &&
                        abs(other.cx-token.cx)<=max(other.right-other.left,token.right-token.left)*0.7 &&
                        normalize(other.text).let { n -> n.startsWith("ADITIV") || Regex("^S[ -]?(10|500)$").matches(n) }
                }.minByOrNull { it.top }
                if (suffix!=null) { text+=" "+normalize(suffix.text);box=Token(text,minOf(token.left,suffix.left),token.top,maxOf(token.right,suffix.right),suffix.bottom) }
            }
            var fuel=product(text)
            val additiveCaption = text.length in 7..10 && text.all { it in 'A'..'Z' } &&
                minOf(levenshtein(text,"ADITIVADO"),levenshtein(text,"ADITIVADA"))<=1
            if ((text.startsWith("ADITIV") || additiveCaption || text == "GRID") && !text.contains("GASOLINA") && !text.contains("DIESEL") && !text.contains("ETANOL")) {
                val context=labelTokens.filter { other -> other!=token && other.top <= token.top && normalize(other.text).let { it.contains("GASOLINA") || it.contains("DIESEL") || it.contains("ETANOL") || Regex("^E ?GRID(?:\\b|$)").containsMatchIn(it) || uncertainProduct(it)!=null } && abs(other.cy-token.cy)<=max(other.height,token.height)*2.5 && abs(other.cx-token.cx)<=max(other.right-other.left,token.right-token.left)*0.7 }.minByOrNull { abs(it.cy-token.cy) }
                context?.let { parent ->
                    var parentText=normalize(parent.text)
                    if (parentText.contains("DIESEL")) {
                        val spec = labelTokens.filter { other ->
                            Regex("^S[ -]?(10|500)$").matches(normalize(other.text)) &&
                                other.top >= parent.top && other.top-parent.bottom <= parent.height*0.8 &&
                                abs(other.cx-parent.cx) <= max(other.right-other.left,parent.right-parent.left)*0.7
                        }.minByOrNull { abs(it.cy-parent.cy) }
                        spec?.let { parentText += " " + normalize(it.text) }
                    }
                    fuel=if(parentText.contains("GASOLINA")) FuelProduct.GASOLINE_PREMIUM else product(parentText)
                    if (uncertainProduct(parentText)!=null) box=box.copy(text=parentText)
                }
            }
            box to fuel
        }.distinct().let { found ->
            found.filterNot { (token,fuel) -> found.any { (parent,parentFuel) -> parent!=token && parentFuel!=null && parentFuel!=fuel && encloses(parent,token) && parent.text.length>token.text.length } ||
                found.any { (parent,parentFuel) -> fuel==FuelProduct.GASOLINE_REGULAR && parentFuel==FuelProduct.GASOLINE_PREMIUM && normalize(parent.text).replace(" ", "").startsWith("GGRID") && token.height<parent.height*0.5 && token.top>=parent.top && token.top-parent.bottom<=parent.height && abs(token.left-parent.left)<=parent.height*2.5 } }
        }
        fun nonPrice(token: Token) = normalize(token.text).let { n ->
            n.contains("TOTAL") || n.contains("LITROS") || n.contains("VOLUME") }
        val values = tokens.filterNot { token -> nonPrice(token) ||
            tokens.any { parent -> parent != token && nonPrice(parent) && encloses(parent,token) } }
            .filterNot { token -> tokens.any { child -> child!=token && encloses(token,child) && price.containsMatchIn(child.text) && child.text.length<token.text.length } }
            .flatMap { token ->
                val rawText = token.text.trim()
                val text = (if (Regex("^[0-9]{1,2} +[0-9]{2,3}$").matches(rawText))
                    rawText.replace(Regex(" +"), ",") else rawText)
                    .replace(Regex("([.,]) +(?=[0-9])"), "$1")
                val raws = price.findAll(text).map { it.value }.toMutableList()
                if (raws.isEmpty()) {
                    // Bare 3-digit whole token only ("675" -> "6,75"). Never
                    // a fragment: S500/S10 specs and years stay untouched.
                    val trimmed = text.trim()
                    if (trimmed.length == 3 && trimmed.all { it in '0'..'9' }) {
                        raws += trimmed.substring(0, 1) + "," + trimmed.substring(1)
                    }
                }
                raws.mapNotNull { raw ->
                    PortablePriceOcr.parseCandidates(raw).singleOrNull()?.let { token to it.priceMilli }
                }
            }
        val matches = mutableListOf<Row>()
        val uncertainMatches = mutableMapOf<FuelProduct, MutableList<Long>>()
        val orphans = mutableListOf<Long>()
        var unresolved = anchors.any { (_, fuel) -> fuel == null }
        for ((value, amount) in values) {
            val ranked = anchors.mapNotNull { (label, fuel) ->
                val h = max(label.height, value.height).toDouble()
                val inline = label == value
                val sameRow = abs(label.cy - value.cy) / h <= 0.75 && value.left >= label.right - h
                val below = !price.containsMatchIn(label.text) && value.top >= label.bottom - h * 0.2 && (value.top - label.bottom) / h <= 2.0 && abs(label.cx - value.cx) <= max(label.right-label.left,value.right-value.left) * 0.7 && anchors.none { (other,_) -> other!=label && other.top>label.bottom && other.top<value.cy && abs(other.cx-label.cx)<max(other.right-other.left,label.right-label.left) }
                val score = when {
                    inline -> 0.0
                    sameRow -> abs(label.cy - value.cy) / h + (value.left-label.right).coerceAtLeast(0) / (h*20)
                    below -> 1.0 + (value.top-label.bottom).coerceAtLeast(0) / h + abs(label.cx-value.cx)/(h*10)
                    else -> null
                }
                score?.let { Triple(fuel,it,label) }
            }.sortedBy { it.second }
            val best = ranked.firstOrNull()
            if (best == null) {
                // Priced value with no recognizable label nearby (the name
                // itself was misread beyond recovery): keep it for manual
                // fuel choice instead of silently dropping it.
                unresolved = true
                if (amount !in orphans && orphans.size < MAX_ORPHANS) orphans += amount
                continue
            }
            // Ambiguous geometry never auto-assigns — but the value is
            // kept for manual choice instead of being dropped.
            if (ranked.drop(1).any { it.first != best.first && abs(it.second-best.second)<0.35 }) {
                unresolved = true
                if (amount !in orphans && orphans.size < MAX_ORPHANS) orphans += amount
                continue
            }
            val fuel=best.first
            if(fuel==null){
                unresolved=true
                uncertainProduct(normalize(best.third.text))?.let { possible ->
                    uncertainMatches.getOrPut(possible) { mutableListOf() } += amount
                }
                if (amount !in orphans && orphans.size < MAX_ORPHANS) orphans += amount
                continue
            }
            matches += Row(fuel,amount)
        }
        // A low-confidence priced label may veto another view, never create a row.
        val conflicts = uncertainMatches.keys.toMutableSet()
        val rows = matches.groupBy { it.product }.mapNotNull { (fuel, found) ->
            val possible=uncertainMatches[fuel].orEmpty()
            val amounts=(found.map{it.amountMilli}+possible).distinct()
            if(amounts.size==1 && possible.isEmpty()) Row(fuel,amounts.single()) else {
                unresolved=true
                conflicts += fuel
                amounts.forEach { if (it !in orphans && orphans.size < MAX_ORPHANS) orphans += it }
                null
            }
        }
        return Result(rows,conditional,unresolved,orphans,conflicts)
    }

    /** Distributor boards can place the E/G icon in a separate text element. */
    private fun recoverBrandHeadings(tokens: List<Token>): List<Token> = tokens.filter {
        normalize(it.text.trim()) == "GRID"
    }.mapNotNull { brand ->
        val prefixes = tokens.filter { prefix ->
            prefix.text.trim().uppercase() in setOf("E", "G") && prefix.right <= brand.left &&
                brand.left-prefix.right <= max(prefix.height,brand.height)*3 &&
                abs(prefix.cy-brand.cy) <= max(prefix.height,brand.height)*0.75
        }
        if (prefixes.size != 1) null else prefixes.single().let { prefix ->
            Token(prefix.text.trim().uppercase()+" GRID",prefix.left,minOf(prefix.top,brand.top),
                brand.right,maxOf(prefix.bottom,brand.bottom))
        }
    }

    /** Some physical boards put integer and fraction in separate OCR lines. */
    private fun recoverSplitPrices(tokens: List<Token>): List<Token> {
        val labels = tokens.filter { labelLike(normalize(it.text)) && !price.containsMatchIn(it.text) }
        if (labels.isEmpty()) return emptyList()
        val integers = tokens.filter { integerFragment.matches(it.text.trim()) }
        val fractions = tokens.filter { fractionFragment.matches(it.text.trim()) }
        val completePrices = tokens.filter { price.containsMatchIn(it.text) }
        val pairs = integers.flatMap { integer ->
            fractions.mapNotNull { fraction ->
                val h = max(integer.height, fraction.height).toDouble()
                val gap = fraction.left - integer.right
                val aligned = abs(integer.cy - fraction.cy) <= h * 0.25 &&
                    minOf(integer.height, fraction.height) >= h * 0.6 && gap >= 0 && gap <= h * 1.25
                if (!aligned) return@mapNotNull null
                val labeled = labels.any { label ->
                    abs(label.cy - integer.cy) <= max(label.height, integer.height) * 0.75 &&
                        integer.left >= label.right && integer.left - label.right <= h * 4
                }
                if (!labeled) return@mapNotNull null
                val complete = completePrices.any { token ->
                    token.left <= integer.left + 2 &&
                        token.top <= minOf(integer.top, fraction.top) + 2 &&
                        token.right >= fraction.right - 2 && token.bottom >= maxOf(integer.bottom, fraction.bottom) - 2
                }
                if (!complete) integer to fraction else null
            }
        }
        return pairs.filter { (integer, fraction) ->
            pairs.count { it.first == integer } == 1 && pairs.count { it.second == fraction } == 1
        }.map { (integer, fraction) ->
            Token(integer.text.trim().take(1) + "," + fraction.text.trim(), integer.left,
                minOf(integer.top, fraction.top), fraction.right, maxOf(integer.bottom, fraction.bottom))
        }
    }

    private fun normalize(text: String): String = text.uppercase()
        .replace('Á','A').replace('À','A').replace('Ã','A').replace('Â','A')
        .replace('É','E').replace('Ê','E').replace('Í','I').replace('Ó','O').replace('Õ','O')
        .replace('Ô','O').replace('Ú','U').replace('Ç','C').replace('Ī','I').replace('İ','I')

    private fun labelLike(text: String): Boolean = product(text)!=null || uncertainProduct(text)!=null || text.contains("DIESEL") || text.contains("PODIUM") || text.contains("PREMIUM") || Regex("\\bS[ -]?[0-9]+\\b").containsMatchIn(text)

    /** One step beyond assignment tolerance may withhold a conflict, never assign a fuel. */
    private fun uncertainProduct(text: String): FuelProduct? {
        if (product(text)!=null || text.length !in 4..10 || text.any { it !in 'A'..'Z' }) return null
        val threshold=if(text.length<=6) 1 else 2
        val ranked=FUZZY_FUELS.map { (word,fuel) -> levenshtein(text,word) to fuel }
        val best=ranked.minOf { it.first }
        if(best!=threshold+1) return null
        return ranked.filter { it.first==best }.map { it.second }.distinct().singleOrNull()
    }

    private fun product(text: String): FuelProduct? = when {
        text.contains("PODIUM") || text.contains("PREMIUM") || text.contains("RACING") -> null
        text.contains("DIESEL") || Regex("\\bS\\s*[- ]?\\s*(10|500)\\b").containsMatchIn(text) -> when {
            Regex("(?:\\b|DIESEL)S\\s*[- ]?\\s*500\\b").containsMatchIn(text) -> FuelProduct.DIESEL_S500
            Regex("(?:\\b|DIESEL)S\\s*[- ]?\\s*10\\b").containsMatchIn(text) -> FuelProduct.DIESEL_S10
            // An explicit unknown/truncated specification must not become bare Diesel.
            Regex("(?:\\b|DIESEL)S\\s*[- ]?\\s*[0-9IO][A-Z0-9]*\\b").containsMatchIn(text) -> null
            // Bare "Diesel" follows the existing common-S500 review contract.
            else -> FuelProduct.DIESEL_S500
        }
        text.contains("ETANOL") || text.contains("ALCOOL") || Regex("^E ?GRID(?:\\b|$)").containsMatchIn(text) -> FuelProduct.ETHANOL
        text.contains("ADITIV") || text.contains("V-POWER") || Regex("\\bGRID\\b").containsMatchIn(text) ||
            Regex("^G ?GRID(?:\\b|$)").containsMatchIn(text) -> FuelProduct.GASOLINE_PREMIUM
        text.contains("GASOLINA") -> FuelProduct.GASOLINE_REGULAR
        Regex("\\bGNV\\b").containsMatchIn(text) || Regex("\\bGAS NATURAL\\b").containsMatchIn(text) -> FuelProduct.CNG
        else -> fuzzyProduct(text)
    }

    /**
     * Near-miss label recovery for single-word OCR misreads ("ETONOL",
     * "GOSOLINO", "DISEL"). Small edit distance against known fuel words
     * only; diesel specs are excluded (S50 is not S500) while a bare
     * "DIESEL" typo follows the common-S500 rule. Multi-word tokens keep
     * the exact contains-rules above. A merged longer anchor still
     * filters the standalone token via the enclosure rule, so this never
     * splits an already-associated row.
     */
    private fun fuzzyProduct(text: String): FuelProduct? {
        if (' ' in text || text.length !in 4..10 || text.any { it !in 'A'..'Z' }) return null
        val threshold = if (text.length <= 6) 1 else 2
        return FUZZY_FUELS.asSequence()
            .map { (word, fuel) -> levenshtein(text, word) to fuel }
            .filter { (distance, _) -> distance in 1..threshold }
            .minByOrNull { (distance, _) -> distance }?.second
    }

    private val FUZZY_FUELS = listOf(
        "ETANOL" to FuelProduct.ETHANOL,
        "ALCOOL" to FuelProduct.ETHANOL,
        "GASOLINA" to FuelProduct.GASOLINE_REGULAR,
        "ADITIVADA" to FuelProduct.GASOLINE_PREMIUM,
        "ADITIVADO" to FuelProduct.GASOLINE_PREMIUM,
        // OCR/board typo of bare Diesel, which is common S500.
        "DIESEL" to FuelProduct.DIESEL_S500,
    )

    private fun levenshtein(a: String, b: String): Int {
        if (a == b) return 0
        var prev = IntArray(b.length + 1) { it }
        var curr = IntArray(b.length + 1)
        for (i in 1..a.length) {
            curr[0] = i
            for (j in 1..b.length) {
                curr[j] = minOf(prev[j] + 1, curr[j - 1] + 1, prev[j - 1] + if (a[i - 1] == b[j - 1]) 0 else 1)
            }
            val tmp = prev; prev = curr; curr = tmp
        }
        return prev[b.length]
    }
}
