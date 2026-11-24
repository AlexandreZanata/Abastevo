import Foundation
import XCTest
import CoreGraphics
import ImageIO
@testable import AnpFuelCore

final class VehicleEditingTests: XCTestCase {
    private let city = Municipality(ibgeCode: "3550308", name: "São Paulo", state: .SP)
    private let station = "11222333000181"
    private let account = VehicleLocalScope(accountId: "account-a", origin: "https://abastevo.com.br")

    func testEditPreservesIdentityAndEstimatesAtTheVehicleLimit() throws {
        let original = VehicleRecord(
            id: "vehicle-existing", name: "Before", propulsion: .flex, primaryFuel: .ethanol, tankCapacityMilliLiters: 48_000,
            gasolineKmPerLiter: 14, ethanolKmPerLiter: 9, whPerKm: 151
        )
        let admission = VehicleAdmissionRule.decide(scope: .local, authority: .proofStale, registeredBefore: 3, newDistinctIds: 0)
        XCTAssertEqual(admission, .accept)
        XCTAssertEqual(
            VehicleAdmissionRule.decide(scope: .local, authority: .serverVerified, registeredBefore: 3, newDistinctIds: 1),
            .capacityExceeded
        )
        var draft = VehicleEditorDraft(existing: original)
        draft.name = "  Edited  "
        draft.propulsion = .plugInHybrid
        draft.tankLiters = "48.123"
        draft.batteryKwh = "12,501"
        draft.connector = .type2
        let result = try draft.build(existing: original).vehicle
        XCTAssertEqual(result.id, original.id)
        XCTAssertEqual(result.name, "Edited")
        XCTAssertEqual(result.primaryFuel, .ethanol)
        XCTAssertEqual(result.tankCapacityMilliLiters, 48_123)
        XCTAssertEqual(result.batteryCapacityWh, 12_501)
        XCTAssertEqual(result.chargeConnector, .type2)
        XCTAssertEqual(result.gasolineKmPerLiter, original.gasolineKmPerLiter)
        XCTAssertEqual(result.ethanolKmPerLiter, original.ethanolKmPerLiter)
        XCTAssertEqual(result.whPerKm, original.whPerKm)

        let legacy = try JSONEncoder().encode(original)
        XCTAssertNil(try JSONDecoder().decode(VehicleRecord.self, from: legacy).chargeConnector)
    }

    func testEditorRefusesMissingRequiredFieldsAndPreventsFuelAlertsOnBEV() throws {
        var draft = VehicleEditorDraft()
        XCTAssertThrowsError(try draft.build(existing: nil)) {
            XCTAssertEqual($0 as? VehicleEditorError, .nameRequired)
        }
        draft.name = "Example"
        draft.propulsion = .plugInHybrid
        draft.tankLiters = "200.001"
        XCTAssertThrowsError(try draft.build(existing: nil)) {
            XCTAssertEqual($0 as? VehicleEditorError, .invalidTank)
        }
        draft.tankLiters = "50"
        draft.batteryKwh = "12,0009"
        XCTAssertThrowsError(try draft.build(existing: nil)) {
            XCTAssertEqual($0 as? VehicleEditorError, .invalidBattery)
        }
        draft.batteryKwh = "45"
        draft.priceMode = .specific
        XCTAssertThrowsError(try draft.build(existing: nil)) {
            XCTAssertEqual($0 as? VehicleEditorError, .stationRequired)
        }
        draft.stationCNPJ = "11.222.333/0001-81"
        draft.priceDropAlertEnabled = true
        XCTAssertEqual(try draft.build(existing: nil).preference.stationCNPJ, station)
        draft.propulsion = .electric
        let electric = try draft.build(existing: nil)
        XCTAssertEqual(electric.vehicle.tankCapacityMilliLiters, 0)
        XCTAssertEqual(electric.vehicle.batteryCapacityWh, 45_000)
        XCTAssertFalse(electric.preference.priceDropAlertEnabled)
        XCTAssertNil(electric.preference.stationCNPJ)
    }

    func testSanitizedPhotoSurvivesRestartWithoutOrientationExifOrLocationMetadata() throws {
        let source = try syntheticPhoto()
        let input = try XCTUnwrap(CGImageSourceCreateWithData(source as CFData, nil))
        let inputProperties = try XCTUnwrap(CGImageSourceCopyPropertiesAtIndex(input, 0, nil) as? [CFString: Any])
        XCTAssertNotNil(inputProperties[kCGImagePropertyExifDictionary])
        XCTAssertNotNil(inputProperties[kCGImagePropertyGPSDictionary])

        let image = try VehicleLocalPhoto(sanitizing: source)
        let data = try XCTUnwrap(CGImageSourceCreateWithData(image.jpeg as CFData, nil))
        let properties = try XCTUnwrap(CGImageSourceCopyPropertiesAtIndex(data, 0, nil) as? [CFString: Any])
        let dimensions = try XCTUnwrap(IosImageIoPhotoCodec().probeDims(bytes: image.jpeg))
        XCTAssertEqual(max(dimensions.width, dimensions.height), 1024)
        XCTAssertGreaterThan(dimensions.height, dimensions.width)
        let exif = properties[kCGImagePropertyExifDictionary] as? [CFString: Any]
        XCTAssertNil(exif?[kCGImagePropertyExifUserComment])
        XCTAssertTrue((properties[kCGImagePropertyGPSDictionary] as? [CFString: Any])?.isEmpty ?? true)
        XCTAssertThrowsError(try VehicleLocalPhoto(sanitizing: Data()))

        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = VehicleDetailsStore(directory: dir)
        let preference = VehiclePricePreference(priceDropAlertEnabled: true)
        try store.save(id: "id", scope: account, preference: preference, photo: .replace(image))
        let restarted = VehicleDetailsStore(directory: dir)
        XCTAssertEqual(try restarted.load(id: "id", scope: account).jpeg, image.jpeg)
        XCTAssertNil(try restarted.load(id: "id", scope: VehicleLocalScope(accountId: nil, origin: account.origin)).jpeg)
        XCTAssertNil(try restarted.load(id: "id", scope: VehicleLocalScope(accountId: "other", origin: account.origin)).jpeg)
        XCTAssertNil(try restarted.load(id: "id", scope: VehicleLocalScope(accountId: account.accountId, origin: "https://other.example")).jpeg)
        try restarted.save(id: "id", scope: account, preference: preference, photo: .remove)
        XCTAssertNil(try store.load(id: "id", scope: account).jpeg)
        XCTAssertTrue(try store.load(id: "id", scope: account).preference.priceDropAlertEnabled)

        try restarted.save(id: "id", scope: account, preference: preference, photo: .replace(image))
        try restarted.remove(id: "id")
        XCTAssertEqual(try store.load(id: "id", scope: account), VehicleLocalDetails())
        try store.save(id: "next", scope: account, preference: preference, photo: .replace(image))
        try store.purge()
        XCTAssertEqual(try restarted.load(id: "next", scope: account), VehicleLocalDetails())
    }

    func testSpecificStationAndCheapestFallbackCompareExactFuelOncePerVehicleAndWeek() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = VehicleDetailsStore(directory: dir)
        var draft = VehicleEditorDraft()
        draft.name = "Vehicle"
        draft.priceDropAlertEnabled = true
        draft.priceMode = .specific
        draft.stationCNPJ = station
        let result = try draft.build(existing: nil)
        try store.save(id: result.vehicle.id, scope: account, preference: result.preference, photo: .keep)
        let details = try store.load(id: result.vehicle.id, scope: account)
        let week = try XCTUnwrap(AnpSurveyWeekRef(startIso: "2026-06-14", endIso: "2026-06-20"))
        let prior = try XCTUnwrap(AnpSurveyWeekRef(startIso: "2026-06-07", endIso: "2026-06-13"))
        let average = AnpMunicipalityAverage(
            state: .SP, municipality: city.name, product: .gasolineRegular, stationCount: 3, unit: "L",
            averageMilli: 6000, minimumMilli: 4700, maximumMilli: 7000
        )
        let current = AnpWeekRows(week: week, averages: [average], stations: [
            price(cnpj: station, fuel: .gasolineRegular, amount: 5700),
            price(cnpj: "12345678000195", fuel: .gasolineRegular, amount: 5400),
            price(cnpj: station, fuel: .ethanol, amount: 2000),
        ])
        let previous = AnpWeekRows(week: prior, averages: [], stations: [
            price(cnpj: "12345678000195", fuel: .gasolineRegular, amount: 5900),
        ])
        let drop = try XCTUnwrap(VehiclePriceDropRule.detect(
            vehicle: result.vehicle, details: details, city: city, current: current, previous: previous
        ))
        XCTAssertEqual(drop.currentMilli, 5700)
        XCTAssertEqual(drop.previousMilli, 5900)
        XCTAssertNil(VehiclePriceDropRule.detect(vehicle: result.vehicle, details: details, city: city, current: current, previous: nil))
        XCTAssertTrue(try store.recordNotified(id: result.vehicle.id, scope: account, week: week.id, matching: result.preference))
        let restarted = try VehicleDetailsStore(directory: dir).load(id: result.vehicle.id, scope: account)
        XCTAssertNil(VehiclePriceDropRule.detect(vehicle: result.vehicle, details: restarted, city: city, current: current, previous: previous))
        let withoutStationRows = AnpWeekRows(week: week, averages: [average])
        let fallback = VehiclePriceDropRule.detect(
            vehicle: result.vehicle, details: details, city: city, current: withoutStationRows, previous: previous
        )
        XCTAssertEqual(fallback?.currentMilli, 4700)
        XCTAssertFalse(try store.recordNotified(
            id: result.vehicle.id, scope: account, week: week.id, matching: VehiclePricePreference()
        ))
    }

    private func price(cnpj: String, fuel: FuelProduct, amount: Int64) -> AnpStationWeekPrice {
        AnpStationWeekPrice(
            cnpj: cnpj, tradeName: nil, legalName: nil, brand: nil, address: "",
            municipality: city.name, state: city.state, product: fuel, priceMilli: amount, collectedOnIso: nil
        )
    }

    private func syntheticPhoto() throws -> Data {
        let canvas = try XCTUnwrap(CGContext(
            data: nil, width: 1200, height: 32, bitsPerComponent: 8, bytesPerRow: 0,
            space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
        ))
        canvas.setFillColor(red: 0.2, green: 0.4, blue: 0.7, alpha: 1)
        canvas.fill(CGRect(x: 0, y: 0, width: 1200, height: 32))
        let photo = try XCTUnwrap(canvas.makeImage())
        let buffer = NSMutableData()
        let destination = try XCTUnwrap(CGImageDestinationCreateWithData(buffer as CFMutableData, "public.jpeg" as CFString, 1, nil))
        let properties: [CFString: Any] = [
            kCGImagePropertyOrientation: 6,
            kCGImagePropertyExifDictionary: [kCGImagePropertyExifUserComment: "synthetic metadata"],
            kCGImagePropertyGPSDictionary: [
                kCGImagePropertyGPSVersion: [2, 3, 0, 0], kCGImagePropertyGPSMapDatum: "WGS-84",
            ],
        ]
        CGImageDestinationAddImage(destination, photo, properties as CFDictionary)
        XCTAssertTrue(CGImageDestinationFinalize(destination))
        return buffer as Data
    }
}
