import Foundation
import AnpFuelCore

/// Where the official prices on screen came from.
public enum OfficialPriceSource: Equatable {
    case live
    case cached(Date)
    case anpDownload(AnpSurveyWeekRef)
}

public struct OfficialPriceRead {
    public let prices: [OfficialStationPrice]
    public let source: OfficialPriceSource
}

extension AppStateViewModel {
    /// Live station prices; when the API is unreachable, the last saved read for this
    /// origin, else the downloaded ANP week matched by CNPJ. Each fallback keeps its date.
    func officialPriceRead(for station: StationRecord) async throws -> OfficialPriceRead {
        let origin = snapshot.apiEnvironment.origin
        do {
            let prices = try await officialPrices(for: station)
            publicReadCache.saveStationPrices(
                origin: origin, stationId: station.id, municipalityCode: station.municipalityCode, prices: prices
            )
            return OfficialPriceRead(prices: prices, source: .live)
        } catch is CancellationError {
            throw CancellationError()
        } catch {
            if let cached = publicReadCache.stationPrices(origin: origin, stationId: station.id) {
                return OfficialPriceRead(
                    prices: cached.prices.map(\.officialPrice),
                    source: .cached(Date(timeIntervalSince1970: TimeInterval(cached.savedAtEpochSeconds)))
                )
            }
            if let week = anpSurvey.displayWeek {
                let rows = anpSurvey.stationWeekPrices(cnpj: station.cnpj)
                if !rows.isEmpty {
                    return OfficialPriceRead(prices: AnpOfficialRules.officialPrices(rows, week: week), source: .anpDownload(week))
                }
            }
            throw error
        }
    }

    /// "Salvo há 2 horas" style label for cached public reads.
    func cachedLabel(_ date: Date) -> String {
        String(localized: "Sem conexão com o servidor: dados salvos \(AnpWeekText.age(since: date)).")
    }
}
