import Foundation
import XCTest
@testable import AnpFuelCore

/// Ledger record model, migration of the first iOS ledger, exact units, payload wire and form rules.
final class LedgerModelsTests: XCTestCase {
    private let vehicleId = "10000000-0000-4000-8000-000000000001"
    private let recordId = "77777777-7777-4777-8777-777777777777"
    private let zone = "America/Sao_Paulo"
    /// 2026-03-01T12:00:00Z.
    private let instant: Int64 = 1_772_366_400_000

    private func vehicle(_ propulsion: Propulsion = .flex, fuel: FuelProduct = .ethanol) -> VehicleRecord {
        return VehicleRecord(
            id: vehicleId,
            name: "Carro",
            propulsion: propulsion,
            primaryFuel: fuel,
            tankCapacityMilliLiters: 50_000,
            batteryCapacityWh: propulsion.supportsElectric ? 52_000 : 0
        )
    }

    // MARK: Migration of persisted snapshots

    func testFirstLedgerSnapshotDecodesForwardWithoutLosingEntries() throws {
        let fresh = try JSONEncoder().encode(PersistedAppSnapshot(vehicles: [vehicle()]))
        var object = try XCTUnwrap(try JSONSerialization.jsonObject(with: fresh) as? [String: Any])
        let legacyEntries: [[String: Any]] = [
            ["id": "a1", "vehicleId": vehicleId, "kind": "REFUEL", "dateLabel": "10/03/2026", "monthKey": "2026-03",
             "odometerKm": 12_345, "quantityMilli": 40_500, "totalCentavos": 23_000, "note": "posto"],
            ["id": "a2", "vehicleId": vehicleId, "kind": "CHARGE", "dateLabel": "11/03/2026", "monthKey": "2026-03",
             "odometerKm": 12_400, "quantityMilli": 0, "totalCentavos": 1_500, "note": ""],
            ["id": "a3", "vehicleId": vehicleId, "kind": "EXPENSE", "dateLabel": "bad", "monthKey": "2026-02",
             "odometerKm": 0, "quantityMilli": 0, "totalCentavos": 9_900, "note": "lavagem"],
            ["id": "a4", "vehicleId": vehicleId, "kind": "ODOMETER", "dateLabel": "12/03/2026", "monthKey": "2026-03",
             "odometerKm": 12_500, "quantityMilli": 0, "totalCentavos": 0],
        ]
        object["ledgerEntries"] = legacyEntries
        let legacy = try JSONSerialization.data(withJSONObject: object)
        let snapshot = try JSONDecoder().decode(PersistedAppSnapshot.self, from: legacy)
        XCTAssertEqual(snapshot.ledgerEntries.count, 4)
        XCTAssertEqual(snapshot.vehicles.count, 1)

        let refuel = snapshot.ledgerEntries[0]
        XCTAssertEqual(refuel.kind, .refuel)
        XCTAssertEqual(refuel.quantityMilli, 40_500)
        XCTAssertEqual(refuel.totalCentavos, 23_000)
        XCTAssertEqual(refuel.fill, .unknown)
        XCTAssertNil(refuel.fuel)
        XCTAssertEqual(refuel.odometerMetres, 12_345_000)
        XCTAssertEqual(refuel.note, "posto")
        XCTAssertTrue(refuel.reviewRequired)
        XCTAssertEqual(refuel.timezone, TimeZone.current.identifier)
        let parts = LedgerTime.components(millis: refuel.occurredAtMillis, timeZone: TimeZone.current)
        XCTAssertEqual(parts.year, 2026)
        XCTAssertEqual(parts.month, 3)
        XCTAssertEqual(parts.day, 10)
        XCTAssertEqual(parts.hour, 12)

        let charge = snapshot.ledgerEntries[1]
        XCTAssertEqual(charge.kind, .charge)
        XCTAssertNil(charge.energyWh, "zero meant not measured in the first ledger")
        XCTAssertEqual(charge.totalCentavos, 1_500)

        let expense = snapshot.ledgerEntries[2]
        XCTAssertEqual(expense.category, .other)
        XCTAssertEqual(expense.totalCentavos, 9_900)
        XCTAssertEqual(LedgerTime.components(millis: expense.occurredAtMillis, timeZone: TimeZone.current).month, 2)

        let reading = snapshot.ledgerEntries[3]
        XCTAssertEqual(reading.kind, .odometer)
        XCTAssertEqual(reading.odometerMetres, 12_500_000)
        XCTAssertNil(reading.dataset)
        XCTAssertFalse(LedgerRecordRules.isSyncable(reading))

        let resolved = LedgerLegacyMigration.resolveFuels(snapshot.ledgerEntries, vehicles: snapshot.vehicles)
        XCTAssertEqual(resolved[0].fuel, .ethanol, "Android derives the ledger fuel from the vehicle")
        XCTAssertFalse(LedgerRecordRules.isSyncable(resolved[0]), "migrated records wait for owner review")
        XCTAssertNil(LedgerLegacyMigration.resolveFuels([refuel], vehicles: [vehicle(.flex, fuel: .lpgP13)])[0].fuel)

        let reencoded = try JSONEncoder().encode(snapshot)
        let again = try JSONDecoder().decode(PersistedAppSnapshot.self, from: reencoded)
        XCTAssertEqual(again.ledgerEntries, snapshot.ledgerEntries, "schema 2 round trips without loss")
    }

    func testSchemaTwoToleratesMissingAndUnknownOptionalFields() throws {
        let json = """
        {"schema":2,"id":"\(recordId)","vehicleId":"\(vehicleId)","kind":"REFUEL","occurredAtMillis":\(instant),
         "timezone":"\(zone)","fuel":"FUTURE_FUEL","fill":"half","quantityMilli":1000,"totalCentavos":500}
        """
        let record = try JSONDecoder().decode(LedgerEntryRecord.self, from: Data(json.utf8))
        XCTAssertNil(record.fuel)
        XCTAssertEqual(record.fill, .unknown)
        XCTAssertFalse(record.opensOdometerSegment)
        XCTAssertFalse(record.missedFillBefore)
        XCTAssertFalse(record.reviewRequired)
        XCTAssertEqual(record.note, "")
    }

    // MARK: Exact units

    func testDecimalInputIsExactAndNeverRounded() {
        XCTAssertEqual(LedgerUnits.parseQuantityMilli("40,5"), 40_500)
        XCTAssertEqual(LedgerUnits.parseQuantityMilli("40.5"), 40_500)
        XCTAssertEqual(LedgerUnits.parseQuantityMilli(" 40,5000 "), 40_500)
        XCTAssertNil(LedgerUnits.parseQuantityMilli("40,5001"))
        XCTAssertNil(LedgerUnits.parseQuantityMilli("0"))
        XCTAssertNil(LedgerUnits.parseQuantityMilli("-1"))
        XCTAssertNil(LedgerUnits.parseQuantityMilli("1.234,5"))
        XCTAssertNil(LedgerUnits.parseQuantityMilli(""))
        XCTAssertNil(LedgerUnits.parseQuantityMilli("1000000,001"))
        XCTAssertEqual(LedgerUnits.parseQuantityMilli("1000000"), 1_000_000_000)
        XCTAssertEqual(LedgerUnits.parseCentavos("123,45"), 12_345)
        XCTAssertEqual(LedgerUnits.parseCentavos("0"), 0)
        XCTAssertNil(LedgerUnits.parseCentavos("1,005"))
        XCTAssertEqual(LedgerUnits.parseKilometres("12345,678", maximumMetres: LedgerBounds.maxDistanceMetres), 12_345_678)
        XCTAssertNil(LedgerUnits.parseKilometres("1000001", maximumMetres: LedgerBounds.maxChargeOdometerMetres))
        XCTAssertEqual(LedgerUnits.parseWh(fromKwh: "52,5"), 52_500)
        XCTAssertEqual(LedgerUnits.parsePercent("100"), 100)
        XCTAssertNil(LedgerUnits.parsePercent("101"))
        XCTAssertNil(LedgerUnits.parsePercent("50,5"))
        XCTAssertNil(LedgerUnits.parseQuantityMilli("99999999999999999999"))
        XCTAssertEqual(LedgerUnits.formatScaled(40_500, fractionDigits: 3), "40,5")
        XCTAssertEqual(LedgerUnits.formatScaled(12_345, fractionDigits: 2), "123,45")
        XCTAssertEqual(LedgerUnits.formatScaled(5, fractionDigits: 3), "0,005")
        XCTAssertEqual(LedgerUnits.formatScaled(1000, fractionDigits: 3), "1")
    }

    func testNotesFollowTheBackendControlAndLengthRules() {
        XCTAssertTrue(LedgerText.isValidNote(String(repeating: "a", count: 280)))
        XCTAssertFalse(LedgerText.isValidNote(String(repeating: "a", count: 281)))
        XCTAssertFalse(LedgerText.isValidNote("linha\nnova"))
        XCTAssertFalse(LedgerText.isValidNote("c1\u{0085}"))
        XCTAssertTrue(LedgerText.isValidNote("família 👨‍👩‍👧"), "format characters such as ZWJ are not controls in Go")
        XCTAssertTrue(LedgerText.isValidTimezone(zone))
        XCTAssertFalse(LedgerText.isValidTimezone("Mars/Base"))
        XCTAssertFalse(LedgerText.isValidTimezone(""))
    }

    func testCanonicalIdsAndNameBasedOperationIds() {
        XCTAssertTrue(LedgerIds.isCanonical(recordId))
        XCTAssertTrue(LedgerIds.isCanonical("a0000000-0000-4000-8000-00000000000b"))
        XCTAssertFalse(LedgerIds.isCanonical("A0000000-0000-4000-8000-00000000000B"), "uppercase hex is not canonical")
        XCTAssertFalse(LedgerIds.isCanonical("00000000-0000-0000-0000-000000000000"))
        XCTAssertFalse(LedgerIds.isCanonical("a1"))
        XCTAssertTrue(LedgerIds.isCanonical(LedgerIds.newId()))
        // Java `UUID.nameUUIDFromBytes` (Android `LedgerCommandFactory`), computed independently.
        XCTAssertEqual(
            LedgerIds.nameBased("ledger-command|a0000000-0000-4000-8000-000000000001|https://abastevo.com.br|refuel|77777777-7777-4777-8777-777777777777|0|{}"),
            "2c40ba46-baad-3d14-8faa-a609409dd956"
        )
        XCTAssertEqual(
            LedgerIds.nameBased("ledger-delete|a0000000-0000-4000-8000-000000000001|https://abastevo.com.br|expense|77777777-7777-4777-8777-777777777777"),
            "402b8f5b-cd25-3033-9872-700c02db353d"
        )
    }

    // MARK: Payload wire

    private func refuel(note: String = "", price: Int64? = nil, odometer: Int64? = 12_345_000) -> LedgerEntryRecord {
        return LedgerEntryRecord(
            id: recordId, vehicleId: vehicleId, kind: .refuel, occurredAtMillis: instant, timezone: zone,
            fuel: .ethanol, quantityMilli: 40_000, totalCentavos: 23_000, unitPriceMilli: price, fill: .full,
            odometerMetres: odometer, note: note
        )
    }

    func testPayloadBodiesKeepAndroidKeyOrderAndOmitAbsentValues() throws {
        XCTAssertEqual(
            LedgerWire.payloadJson(refuel(note: "posto \"A\"", price: 5_750)),
            "{\"id\":\"\(recordId)\",\"vehicle_id\":\"\(vehicleId)\",\"occurred_at\":\"2026-03-01T12:00:00Z\",\"timezone\":\"America/Sao_Paulo\",\"fuel\":\"ETHANOL\",\"unit\":\"L\",\"quantity_milli\":40000,\"total_centavos\":23000,\"unit_price_milli\":5750,\"odometer_metres\":12345000,\"fill\":\"full\",\"note\":\"posto \\\"A\\\"\"}"
        )
        let expense = LedgerEntryRecord(
            id: recordId, vehicleId: vehicleId, kind: .expense, occurredAtMillis: instant + 500, timezone: zone,
            totalCentavos: 9_900, category: .tolls
        )
        XCTAssertEqual(
            LedgerWire.payloadJson(expense),
            "{\"id\":\"\(recordId)\",\"vehicle_id\":\"\(vehicleId)\",\"occurred_at\":\"2026-03-01T12:00:00.500Z\",\"timezone\":\"America/Sao_Paulo\",\"amount_centavos\":9900,\"category\":\"tolls\"}"
        )
        let charge = LedgerEntryRecord(
            id: recordId, vehicleId: vehicleId, kind: .charge, occurredAtMillis: instant, timezone: zone,
            energyWh: 15_000, socStartPercent: 20, socEndPercent: 80
        )
        XCTAssertEqual(
            LedgerWire.payloadJson(charge),
            "{\"id\":\"\(recordId)\",\"vehicle_id\":\"\(vehicleId)\",\"occurred_at\":\"2026-03-01T12:00:00Z\",\"timezone\":\"America/Sao_Paulo\",\"energy_wh\":15000,\"soc_start_percent\":20,\"soc_end_percent\":80}"
        )
        for body in [LedgerWire.payloadJson(refuel()), LedgerWire.payloadJson(expense), LedgerWire.payloadJson(charge)] {
            let data = Data(try XCTUnwrap(body).utf8)
            XCTAssertNotNil(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        }
        XCTAssertNil(LedgerWire.payloadJson(refuel(note: "a\u{0007}")))
        XCTAssertNil(LedgerWire.payloadJson(LedgerEntryRecord(vehicleId: vehicleId, kind: .odometer, occurredAtMillis: instant, timezone: zone)))
    }

    func testServerPayloadsDecodeGoCanonicalShapeAndRefuseUnknowns() throws {
        // Go marshals absent odometers as null and omits empty optional strings and zero unit prices.
        let goRefuel = """
        {"id":"\(recordId)","vehicle_id":"\(vehicleId)","occurred_at":"2026-03-01T09:00:00-03:00","timezone":"America/Sao_Paulo","fuel":"ETHANOL","unit":"L","quantity_milli":40000,"total_centavos":23000,"odometer_metres":null,"fill":"partial"}
        """
        let decoded = try XCTUnwrap(LedgerWire.decodePayload(json: goRefuel, dataset: .refuel))
        XCTAssertEqual(decoded.occurredAtMillis, instant)
        XCTAssertNil(decoded.odometerMetres)
        XCTAssertEqual(decoded.fill, .partial)
        XCTAssertEqual(decoded.note, "")
        XCTAssertEqual(
            LedgerWire.payloadJson(decoded),
            "{\"id\":\"\(recordId)\",\"vehicle_id\":\"\(vehicleId)\",\"occurred_at\":\"2026-03-01T12:00:00Z\",\"timezone\":\"America/Sao_Paulo\",\"fuel\":\"ETHANOL\",\"unit\":\"L\",\"quantity_milli\":40000,\"total_centavos\":23000,\"fill\":\"partial\"}"
        )
        let goCharge = """
        {"id":"\(recordId)","vehicle_id":"\(vehicleId)","occurred_at":"2026-03-01T12:00:00Z","timezone":"UTC","energy_wh":null,"total_centavos":1275,"soc_start_percent":null,"soc_end_percent":null,"odometer_metres":null,"fees_centavos":null}
        """
        let charge = try XCTUnwrap(LedgerWire.decodePayload(json: goCharge, dataset: .charge))
        XCTAssertNil(charge.energyWh)
        XCTAssertEqual(charge.totalCentavos, 1_275)

        XCTAssertNil(LedgerWire.decodePayload(json: goRefuel.replacingOccurrences(of: "\"fill\":\"partial\"", with: "\"fill\":\"half\""), dataset: .refuel))
        XCTAssertNil(LedgerWire.decodePayload(json: goRefuel.replacingOccurrences(of: "\"unit\":\"L\"", with: "\"unit\":\"M3\""), dataset: .refuel))
        XCTAssertNil(LedgerWire.decodePayload(json: goRefuel.replacingOccurrences(of: "\"fill\":\"partial\"", with: "\"fill\":\"full\",\"extra\":1"), dataset: .refuel))
        XCTAssertNil(LedgerWire.decodePayload(json: goRefuel.replacingOccurrences(of: "40000", with: "40000.5"), dataset: .refuel))
        XCTAssertNil(LedgerWire.decodePayload(json: goRefuel, dataset: .expense), "a refuel body is not an expense")
    }

    func testFinancialWireVectorsMatchTheBackendAcceptance() throws {
        let lines = LedgerFixtures.financialWire.split(separator: "\n").map(String.init)
        XCTAssertEqual(lines.first, "id,fuel,unit,quantity_milli,total_centavos,accepted")
        XCTAssertEqual(lines.count, 23)
        for line in lines.dropFirst() {
            let cells = line.split(separator: ",", omittingEmptySubsequences: false).map(String.init)
            XCTAssertEqual(cells.count, 6, line)
            let body = "{\"id\":\"\(recordId)\",\"vehicle_id\":\"\(vehicleId)\",\"occurred_at\":\"2026-03-01T12:00:00Z\",\"timezone\":\"UTC\",\"fuel\":\"\(cells[1])\",\"unit\":\"\(cells[2])\",\"quantity_milli\":\(cells[3]),\"total_centavos\":\(cells[4]),\"fill\":\"full\"}"
            let decoded = LedgerWire.decodePayload(json: body, dataset: .refuel)
            XCTAssertEqual(decoded != nil, cells[5] == "1", cells[0])
            if let decoded {
                XCTAssertEqual(LedgerWire.decodePayload(json: try XCTUnwrap(LedgerWire.payloadJson(decoded)), dataset: .refuel), decoded, cells[0])
            }
        }
    }

    func testCsvExportIsFormulaSafe() {
        XCTAssertEqual(LedgerCsvExport.escape("=SUM(A1)"), "\"'=SUM(A1)\"")
        XCTAssertEqual(LedgerCsvExport.escape("a,b"), "\"'a,b\"")
        XCTAssertEqual(LedgerCsvExport.escape("plain"), "plain")
        let csv = LedgerCsvExport.build([refuel(note: "@cmd")])
        let lines = csv.split(separator: "\n").map(String.init)
        XCTAssertEqual(lines.first, LedgerCsvExport.header)
        XCTAssertEqual(lines.count, 2)
        XCTAssertTrue(lines[1].contains("\"'@cmd\""))
        XCTAssertTrue(lines[1].contains(",40000,23000,"))
    }

    // MARK: Entry form (Android `LedgerEntriesViewModel.buildDraft`)

    func testFormRequiresASelectedVehicleAndCompatiblePropulsion() {
        var form = LedgerEntryForm(occurredAtMillis: instant + 123)
        form.quantityText = "40"
        form.totalText = "230"
        XCTAssertEqual(form.build(id: recordId, vehicles: [vehicle()], timezone: zone, existing: nil).failureValue, .vehicleRequired)
        form.vehicleId = vehicleId
        guard case .success(let record) = form.build(id: recordId, vehicles: [vehicle()], timezone: zone, existing: nil) else {
            return XCTFail("refuel should build")
        }
        XCTAssertEqual(record.fuel, .ethanol)
        XCTAssertEqual(record.quantityMilli, 40_000)
        XCTAssertEqual(record.totalCentavos, 23_000)
        XCTAssertEqual(record.occurredAtMillis, instant, "whole seconds only")
        XCTAssertNil(record.odometerMetres, "blank optional fields stay absent")
        XCTAssertNil(record.unitPriceMilli)
        XCTAssertEqual(form.build(id: recordId, vehicles: [vehicle(.electric)], timezone: zone, existing: nil).failureValue, .refuelUnavailable)
        XCTAssertEqual(form.build(id: recordId, vehicles: [vehicle(.flex, fuel: .lpgP13)], timezone: zone, existing: nil).failureValue, .fuelRequired)

        form.kind = .charge
        XCTAssertEqual(form.build(id: recordId, vehicles: [vehicle()], timezone: zone, existing: nil).failureValue, .chargeUnavailable)
        form.energyKwhText = "15"
        form.totalText = ""
        form.socStartText = "80"
        form.socEndText = "101"
        XCTAssertEqual(form.build(id: recordId, vehicles: [vehicle(.electric)], timezone: zone, existing: nil).failureValue, .socInvalid)
        form.socEndText = ""
        guard case .success(let charge) = form.build(id: recordId, vehicles: [vehicle(.electric)], timezone: zone, existing: nil) else {
            return XCTFail("charge should build")
        }
        XCTAssertEqual(charge.energyWh, 15_000)
        XCTAssertNil(charge.totalCentavos)
        XCTAssertEqual(charge.socStartPercent, 80)

        form.kind = .expense
        form.totalText = "1,005"
        XCTAssertEqual(form.build(id: recordId, vehicles: [vehicle()], timezone: zone, existing: nil).failureValue, .totalInvalid)
        form.kind = .odometer
        XCTAssertEqual(form.build(id: recordId, vehicles: [vehicle()], timezone: zone, existing: nil).failureValue, .kindUnsupported)
    }

    func testEditingKeepsIdentityStationAndCompletesReview() throws {
        let station = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
        let migrated = LedgerEntryRecord(
            id: recordId, vehicleId: vehicleId, kind: .refuel, occurredAtMillis: instant, timezone: zone,
            fuel: .ethanol, quantityMilli: 40_500, totalCentavos: 23_000, fill: .unknown, stationId: station,
            odometerMetres: 12_345_000, reviewRequired: true
        )
        var form = LedgerEntryForm.editing(migrated)
        XCTAssertEqual(form.quantityText, "40,5")
        XCTAssertEqual(form.totalText, "230")
        XCTAssertEqual(form.odometerKmText, "12345")
        form.fill = .full
        guard case .success(let saved) = form.build(id: migrated.id, vehicles: [vehicle()], timezone: zone, existing: migrated) else {
            return XCTFail("edit should build")
        }
        XCTAssertEqual(saved.id, recordId)
        XCTAssertEqual(saved.stationId, station)
        XCTAssertEqual(saved.fill, .full)
        XCTAssertFalse(saved.reviewRequired)
        XCTAssertTrue(LedgerRecordRules.isSyncable(saved))
    }
}

private extension Result {
    var failureValue: Failure? {
        if case .failure(let error) = self { return error }
        return nil
    }
}
