import Foundation

/// Portable OCR price parsing, spatial board association, framing and live zoom
/// (P10-T04 / P21-T02 / B-BR-PC02).
///
/// Mirrors `PortablePriceOcr.kt`, `FuelBoardOcr.kt`, `FuelBoardFraming.kt`, and `LiveBoardZoom.kt`.
/// Zero postfix unwrap operators anywhere in this file.
public enum PortablePriceOcr {
    public static let lowConfidenceThreshold: Double = 0.60
    public static let maxCandidates = 10

    public struct OcrCandidate: Equatable, Identifiable {
        public var id: String { "\(raw)-\(priceMilli)" }
        public let priceMilli: Int64
        public let raw: String
        public let confidence: Double
        public let hasCurrencyMarker: Bool
        public let manualEntry: Bool

        public init(
            priceMilli: Int64,
            raw: String,
            confidence: Double,
            hasCurrencyMarker: Bool,
            manualEntry: Bool = false
        ) {
            self.priceMilli = priceMilli
            self.raw = raw
            self.confidence = confidence
            self.hasCurrencyMarker = hasCurrencyMarker
            self.manualEntry = manualEntry
        }
    }

    public static func isLowConfidence(_ candidate: OcrCandidate) -> Bool {
        return !candidate.manualEntry && candidate.confidence < lowConfidenceThreshold
    }

    public static func parseCandidates(_ ocrText: String) -> [OcrCandidate] {
        if ocrText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { return [] }
        var out: [OcrCandidate] = []
        let lines = splitLines(ocrText)
        for line in lines {
            if out.count >= maxCandidates { break }
            let marked = lineHasCurrencyMarker(line)
            let raws = scanPriceRaws(line)
            for raw in raws {
                if out.count >= maxCandidates { break }
                guard let milli = toMilliOrNull(raw) else { continue }
                let confidence = marked ? 0.90 : 0.50
                out.append(
                    OcrCandidate(
                        priceMilli: milli,
                        raw: raw,
                        confidence: confidence,
                        hasCurrencyMarker: marked
                    )
                )
            }
        }
        return out
    }

    private static func splitLines(_ text: String) -> [String] {
        return text
            .replacingOccurrences(of: "\r\n", with: "\n")
            .replacingOccurrences(of: "\r", with: "\n")
            .split(separator: "\n", omittingEmptySubsequences: true)
            .map { String($0) }
    }

    private static func lineHasCurrencyMarker(_ line: String) -> Bool {
        let lower = line.lowercased()
        if lower.contains("r$") || lower.contains("brl") || lower.contains("$") || lower.contains("rs") {
            return true
        }
        return false
    }

    private static func scanPriceRaws(_ line: String) -> [String] {
        let chars = Array(line)
        var raws: [String] = []
        var i = 0
        while i < chars.count {
            if !isDigit(chars[i]) {
                i += 1
                continue
            }
            var j = i
            while j < chars.count && isDigit(chars[j]) { j += 1 }
            let intLen = j - i
            if intLen >= 1 && intLen <= 4 && j < chars.count && (chars[j] == "." || chars[j] == ",") {
                var k = j + 1
                while k < chars.count && isDigit(chars[k]) { k += 1 }
                let fracLen = k - (j + 1)
                if fracLen >= 2 && fracLen <= 3 {
                    raws.append(String(chars[i..<k]))
                    i = k
                    continue
                }
            }
            i = j
        }
        return raws
    }

    private static func toMilliOrNull(_ raw: String) -> Int64? {
        let normalized = raw.replacingOccurrences(of: ".", with: ",")
        return try? PortableMoney.parse(normalized)
    }

    private static func isDigit(_ c: Character) -> Bool {
        return c >= "0" && c <= "9"
    }
}

public enum OcrFuelProduct: String, CaseIterable, Codable, Equatable, Hashable, Sendable {
    case ethanol = "ETHANOL"
    case gasolineRegular = "GASOLINE_REGULAR"
    case gasolinePremium = "GASOLINE_PREMIUM"
    case gasolinePremiumGrade = "GASOLINE_PREMIUM_GRADE"
    case dieselS500 = "DIESEL_S500"
    case dieselS10 = "DIESEL_S10"
    case cng = "CNG"
    case lpgP13 = "LPG_P13"
}

public enum FuelBoardOcr {
    public struct Token: Equatable, Hashable {
        public let text: String
        public let left: Int
        public let top: Int
        public let right: Int
        public let bottom: Int
        public let angle: Double

        public var height: Int { max(1, bottom - top) }
        public var cx: Double { Double(left + right) / 2.0 }
        public var cy: Double { Double(top + bottom) / 2.0 }

        public init(text: String, left: Int, top: Int, right: Int, bottom: Int, angle: Double = 0) {
            self.text = text
            self.left = left
            self.top = top
            self.right = right
            self.bottom = bottom
            self.angle = angle
        }
    }

    public struct Row: Equatable, Hashable {
        public let product: OcrFuelProduct
        public let amountMilli: Int64

        public init(product: OcrFuelProduct, amountMilli: Int64) {
            self.product = product
            self.amountMilli = amountMilli
        }
    }

    public struct Result: Equatable {
        public let rows: [Row]
        public let conditional: Bool
        public let unresolved: Bool
        public let orphans: [Int64]
        public let conflictingProducts: Set<OcrFuelProduct>

        public init(
            rows: [Row],
            conditional: Bool,
            unresolved: Bool,
            orphans: [Int64] = [],
            conflictingProducts: Set<OcrFuelProduct> = []
        ) {
            self.rows = rows
            self.conditional = conditional
            self.unresolved = unresolved
            self.orphans = orphans
            self.conflictingProducts = conflictingProducts
        }
    }

    public static let maxOrphans = 5
    public static let maxViews = 4
    private static let maxSuggestedUnitPriceMilli: Int64 = 15_000

    private static let conditionalWords = [
        "VISTA", "CARTAO", "CREDITO", "DEBITO", "PIX", "APP", "CLUBE", "FIDELIDADE", "PROMOCAO"
    ]

    public static func associate(tokens input: [Token]) -> Result {
        var seen: Set<Token> = []
        var original: [Token] = []
        for token in input.prefix(512) {
            if token.text.count <= 256 && token.right > token.left && token.bottom > token.top && !seen.contains(token) {
                seen.insert(token)
                original.append(token)
            }
        }
        let conditional = original.contains { token in
            let upper = normalizeFuelText(token.text)
            return conditionalWords.contains { word in
                upper.range(of: "\\b\(word)\\b", options: .regularExpression) != nil
            }
        }

        let labels = original.compactMap { token -> (Token, OcrFuelProduct)? in
            guard let product = classifyProduct(normalizeFuelText(token.text)) else { return nil }
            return (token, product)
        }

        var rawPrices: [(Token, Int64)] = []
        for token in original {
            let trimmed = token.text.trimmingCharacters(in: .whitespacesAndNewlines)
            if trimmed.range(of: "^(?:R\\$ ?)?0?0[.,] ?0{2,3}$", options: .regularExpression) != nil {
                continue
            }
            let candidates = PortablePriceOcr.parseCandidates(trimmed)
            if !candidates.isEmpty {
                for candidate in candidates {
                    rawPrices.append((token, candidate.priceMilli))
                }
            } else if trimmed.count == 3 && trimmed.allSatisfy({ $0 >= "0" && $0 <= "9" }) {
                let recovered = "\(trimmed.prefix(1)),\(trimmed.dropFirst())"
                if let milli = try? PortableMoney.parse(recovered) {
                    rawPrices.append((token, milli))
                }
            }
        }

        var rows: [Row] = []
        var orphans: [Int64] = []
        var assignedByFuel: [OcrFuelProduct: Set<Int64>] = [:]
        var unresolved = false

        for (valueToken, amount) in rawPrices {
            if !plausible(amount) {
                unresolved = true
                if !orphans.contains(amount) && orphans.count < maxOrphans {
                    orphans.append(amount)
                }
                continue
            }
            var bestScore = Double.greatestFiniteMagnitude
            var bestProduct: OcrFuelProduct?
            for (labelToken, product) in labels {
                let h = Double(max(labelToken.height, valueToken.height))
                if labelToken == valueToken {
                    bestScore = 0
                    bestProduct = product
                    break
                }
                let sameRow = abs(labelToken.cy - valueToken.cy) / h <= 0.75 && Double(valueToken.left) >= Double(labelToken.right) - h
                let below = Double(valueToken.top) >= Double(labelToken.bottom) - h * 0.2
                    && (Double(valueToken.top) - Double(labelToken.bottom)) / h <= 2.0
                    && abs(labelToken.cx - valueToken.cx) <= Double(max(labelToken.right - labelToken.left, valueToken.right - valueToken.left)) * 0.75
                let score: Double?
                if sameRow {
                    score = abs(labelToken.cy - valueToken.cy) / h + max(0.0, Double(valueToken.left - labelToken.right)) / (h * 20.0)
                } else if below {
                    score = 1.0 + max(0.0, Double(valueToken.top - labelToken.bottom)) / h + abs(labelToken.cx - valueToken.cx) / (h * 10.0)
                } else {
                    score = nil
                }
                if let s = score, s < bestScore {
                    bestScore = s
                    bestProduct = product
                }
            }
            if let fuel = bestProduct {
                var set = assignedByFuel[fuel] ?? []
                set.insert(amount)
                assignedByFuel[fuel] = set
            } else {
                unresolved = true
                if !orphans.contains(amount) && orphans.count < maxOrphans {
                    orphans.append(amount)
                }
            }
        }

        var conflicts: Set<OcrFuelProduct> = []
        for (fuel, amounts) in assignedByFuel {
            if amounts.count == 1, let single = amounts.first {
                rows.append(Row(product: fuel, amountMilli: single))
            } else {
                unresolved = true
                conflicts.insert(fuel)
                for amount in amounts {
                    if !orphans.contains(amount) && orphans.count < maxOrphans {
                        orphans.append(amount)
                    }
                }
            }
        }
        rows.sort { $0.product.rawValue < $1.product.rawValue }
        return Result(
            rows: rows,
            conditional: conditional,
            unresolved: unresolved,
            orphans: orphans,
            conflictingProducts: conflicts
        )
    }

    public static func isFuelLabel(_ text: String) -> Bool {
        return classifyProduct(normalizeFuelText(text)) != nil
    }

    public static func classifyProduct(_ normalized: String) -> OcrFuelProduct? {
        if normalized.contains("ETANOL") || normalized.contains("ALCOOL") {
            return .ethanol
        }
        if normalized.contains("DIESEL") {
            if normalized.contains("S10") || normalized.contains("S 10") || normalized.contains("S-10") {
                return .dieselS10
            }
            return .dieselS500
        }
        if normalized.contains("GNV") || normalized.contains("GAS NATURAL") {
            return .cng
        }
        if normalized.contains("PODIUM") || normalized.contains("PREMIUM") || normalized.contains("OCTAPRO") || normalized.contains("RACING") {
            return .gasolinePremiumGrade
        }
        if normalized.contains("ADITIV") || normalized.contains("GRID") {
            return .gasolinePremium
        }
        if normalized.contains("GASOLINA") {
            return .gasolineRegular
        }
        return nil
    }

    private static func normalizeFuelText(_ text: String) -> String {
        return text.folding(options: [.diacriticInsensitive, .caseInsensitive], locale: Locale(identifier: "pt_BR")).uppercased()
    }

    private static func plausible(_ amountMilli: Int64) -> Bool {
        return amountMilli >= 1_000 && amountMilli < maxSuggestedUnitPriceMilli
    }
}

public enum FuelBoardFraming {
    public struct Frame: Equatable {
        public let left: Int
        public let top: Int
        public let right: Int
        public let bottom: Int
        public let priceHeight: Int
        public let angleDegrees: Double
        public let labeled: Bool

        public var width: Int { right - left }
        public var height: Int { bottom - top }

        public init(left: Int, top: Int, right: Int, bottom: Int, priceHeight: Int, angleDegrees: Double, labeled: Bool) {
            self.left = left
            self.top = top
            self.right = right
            self.bottom = bottom
            self.priceHeight = priceHeight
            self.angleDegrees = angleDegrees
            self.labeled = labeled
        }
    }

    public static let targetPriceHeight = 64
    public static let compactPriceHeight = 40
    public static let maxZoom = 6.0
    public static let minZoom = 0.4
    public static let minSkewDegrees = 2.5
    public static let maxSkewDegrees = 35.0

    public static func locate(tokens: [FuelBoardOcr.Token], width: Int, height: Int) -> Frame? {
        if width <= 0 || height <= 0 { return nil }
        let usable = tokens.prefix(512).filter { $0.right > $0.left && $0.bottom > $0.top && $0.text.count <= 256 }
        let prices = usable.filter { !PortablePriceOcr.parseCandidates($0.text).isEmpty }
        if prices.isEmpty { return nil }
        let labels = usable.filter { FuelBoardOcr.isFuelLabel($0.text) }
        if labels.isEmpty && prices.count < 2 { return nil }
        let box = prices + labels
        let left = max(0, box.map { $0.left }.min() ?? 0)
        let top = max(0, box.map { $0.top }.min() ?? 0)
        let right = min(width, box.map { $0.right }.max() ?? width)
        let bottom = min(height, box.map { $0.bottom }.max() ?? height)
        if right - left < 8 || bottom - top < 8 { return nil }
        let heights = prices.map { $0.height }.sorted()
        let medianHeight = heights[heights.count / 2]
        return Frame(
            left: left,
            top: top,
            right: right,
            bottom: bottom,
            priceHeight: medianHeight,
            angleDegrees: 0.0,
            labeled: !labels.isEmpty
        )
    }

    public static func zoomFor(frame: Frame, maxSide: Int = 2048, digitHeight: Int = targetPriceHeight) -> Double {
        let floorVal = digitHeight >= targetPriceHeight ? 1.0 : minZoom
        let wanted = min(maxZoom, max(floorVal, Double(digitHeight) / Double(max(1, frame.priceHeight))))
        let fit = Double(maxSide) / Double(max(1, max(frame.width, frame.height)))
        return min(wanted, fit)
    }
}

public enum LiveBoardZoom {
    public static let targetFill = 0.8
    public static let edgeLimit = 0.95
    public static let maxRatio = 5.0
    public static let wideRatio = 1.0
    public static let hysteresis = 0.15
    public static let confirmAnalyses = 2
    public static let releaseAnalyses = 6

    public struct Board: Equatable {
        public let left: Double
        public let top: Double
        public let right: Double
        public let bottom: Double

        public init?(left: Double, top: Double, right: Double, bottom: Double) {
            guard left >= 0, top >= 0, right <= 1, bottom <= 1, left < right, top < bottom else {
                return nil
            }
            self.left = left
            self.top = top
            self.right = right
            self.bottom = bottom
        }
    }

    public struct State: Equatable {
        public let ratio: Double
        public let seen: Int
        public let missed: Int

        public init(ratio: Double, seen: Int = 0, missed: Int = 0) {
            self.ratio = ratio
            self.seen = seen
            self.missed = missed
        }
    }

    public static func next(state: State, board: Board?, minRatio: Double, maxRatio: Double) -> State {
        let wide = min(max(wideRatio, minRatio), maxRatio)
        let ceiling = max(wide, min(maxRatio, Self.maxRatio))
        guard let board = board else {
            let missed = min(state.missed + 1, releaseAnalyses)
            return State(ratio: missed >= releaseAnalyses ? wide : state.ratio, seen: 0, missed: missed)
        }
        let seen = min(state.seen + 1, confirmAnalyses)
        if seen < confirmAnalyses {
            return State(ratio: state.ratio, seen: seen, missed: 0)
        }
        let fill = max(board.right - board.left, board.bottom - board.top)
        let extent = 2.0 * max(
            abs(board.left - 0.5),
            abs(board.right - 0.5),
            abs(board.top - 0.5),
            abs(board.bottom - 0.5)
        )
        let rawTarget = state.ratio * min(targetFill / max(0.01, fill), edgeLimit / max(0.01, extent))
        let target = min(ceiling, max(wide, rawTarget))
        let atBound = target == wide || target == ceiling
        let moves = abs(target - state.ratio) / max(0.01, state.ratio) > hysteresis || (atBound && target != state.ratio)
        return State(ratio: moves ? target : state.ratio, seen: seen, missed: 0)
    }
}
