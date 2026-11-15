package com.anpfuel.domain.enterprise.document

import com.anpfuel.domain.enterprise.AllocationLine
import com.anpfuel.domain.enterprise.EnterpriseValues
import java.time.Instant
import kotlin.random.Random
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseDocumentTest {
    private fun id(index: Int) = "9d000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private fun digest(n: Int) = "%02x".format(n and 255).repeat(32)
    private val org = id(1)
    private val other = id(2)
    private fun document(index: Int = 201) = EnterpriseDocument.create(org, id(index), digest(index))
    private fun expense(index: Int, owner: String = org) = DocumentTarget(owner, DocumentTargetKind.EXPENSE, id(index))
    private fun fails(expected: DocumentFailure = DocumentFailure.INVALID, block: () -> Unit) {
        assertEquals(expected, assertThrows(DocumentDomainException::class.java, block).failure)
    }
    private fun EnterpriseDocument.go(dimension: DocumentDimension, from: DocumentState, to: DocumentState, at: Instant? = null) =
        apply(DocumentTransition(dimension, from, to, at))
    private fun stored(index: Int) = document(index)
        .go(DocumentDimension.STORAGE, DocumentState.UPLOADING, DocumentState.RECEIVED)
        .go(DocumentDimension.STORAGE, DocumentState.RECEIVED, DocumentState.STORED)
    private fun state(s: DocumentStatuses, d: DocumentDimension) = when (d) {
        DocumentDimension.STORAGE -> s.storage
        DocumentDimension.EXTRACTION -> s.extraction
        DocumentDimension.STRUCTURE -> s.structure
        DocumentDimension.CONFERENCE -> s.conference
        DocumentDimension.RECONCILIATION -> s.reconciliation
        DocumentDimension.FISCAL -> s.fiscal
    }

    @Test fun `identity is scoped canonical and immutable`() {
        val d = document()
        assertEquals(org, d.organizationId); assertEquals(id(201), d.id); assertEquals(digest(201), d.digest)
        fails(DocumentFailure.SCOPE) { EnterpriseDocument.create("", id(201), digest(1)) }
        fails(DocumentFailure.SCOPE) { EnterpriseDocument.create(org.uppercase(), id(201), digest(1)) }
        for (bad in listOf("", "00000000-0000-0000-0000-000000000000", id(201).uppercase(), "not-a-uuid")) fails { EnterpriseDocument.create(org, bad, digest(1)) }
        for (bad in listOf("", digest(26).uppercase(), digest(26).dropLast(1), digest(26) + "1", "g" + digest(26).drop(1))) {
            fails { EnterpriseDocument.create(org, id(201), bad) }
            fails { d.matchesOriginal(bad) }
        }
        assertTrue(d.matchesOriginal(digest(201))); assertFalse(d.matchesOriginal(digest(202)))
        fails(DocumentFailure.SCOPE) { DocumentTarget("", DocumentTargetKind.EXPENSE, id(9)) }
        fails { DocumentTarget(org, DocumentTargetKind.EXPENSE, id(9).uppercase()) }
        for (raw in listOf("", "MAINTENANCE", "expense", "EXPENSE ")) fails { DocumentTargetKind.parse(raw) }
        assertEquals(listOf(DocumentTargetKind.EXPENSE), DocumentTargetKind.entries.filter { it.allocatable })
    }

    @Test fun `every allowed transition changes exactly one dimension and every refused pair nothing`() {
        var clock = Instant.parse("2026-11-14T12:00:00Z")
        val base = document().withAmount(97350).link(expense(211)).allocate(listOf(DocumentAllocationLine(expense(211), 60000)))
        for (dimension in DocumentDimension.entries.filterNot { it.derived }) {
            val states = DocumentState.entries.filter { it.dimension == dimension }
            val initial = DocumentState.initial(dimension)
            assertEquals(initial, state(base.statuses, dimension))
            val seen = linkedMapOf(initial to base)
            val queue = ArrayDeque(listOf(initial))
            while (queue.isNotEmpty()) {
                val from = queue.removeFirst()
                val current = seen.getValue(from)
                val next = DocumentState.allowed(dimension, from)
                assertEquals(next.sortedBy { it.token }, next)
                for (to in states) {
                    clock = clock.plusSeconds(1)
                    val at = if (dimension == DocumentDimension.FISCAL) clock else null
                    if (to !in next) {
                        fails(DocumentFailure.INVALID_TRANSITION) { current.go(dimension, from, to, at) }
                        continue
                    }
                    val after = current.go(dimension, from, to, at)
                    for (d in DocumentDimension.entries) {
                        assertEquals(if (d == dimension) to else state(current.statuses, d), state(after.statuses, d))
                    }
                    assertEquals(current.digest, after.digest); assertEquals(current.id, after.id)
                    assertEquals(current.links, after.links); assertEquals(current.allocations, after.allocations)
                    assertEquals(current.amount, after.amount)
                    if (dimension != DocumentDimension.FISCAL) assertEquals(current.consultedAt, after.consultedAt)
                    if (to !in seen) { seen[to] = after; queue.addLast(to) }
                }
            }
            val expected = if (dimension == DocumentDimension.STORAGE) states - DocumentState.REPLACED else states
            assertEquals(expected.toSet(), seen.keys)
        }
    }

    @Test fun `reconciliation is derived and transitions name their own dimension`() {
        val d = document()
        assertTrue(DocumentDimension.RECONCILIATION.derived)
        for (from in DocumentState.entries.filter { it.dimension == DocumentDimension.RECONCILIATION }) {
            fails(DocumentFailure.DERIVED) { DocumentState.allowed(DocumentDimension.RECONCILIATION, from) }
            fails(DocumentFailure.DERIVED) { d.go(DocumentDimension.RECONCILIATION, from, DocumentState.RECONCILED) }
        }
        assertEquals(DocumentState.RECEIVED, d.apply(DocumentTransition(DocumentDimension.STORAGE, DocumentState.UPLOADING, DocumentState.RECEIVED)).statuses.storage)
        fails { d.go(DocumentDimension.STORAGE, DocumentState.EXTRACTION_NONE, DocumentState.RECEIVED) }
        fails { d.go(DocumentDimension.STORAGE, DocumentState.UPLOADING, DocumentState.UNCONFERRED) }
        fails { DocumentState.allowed(DocumentDimension.STORAGE, DocumentState.EXTRACTION_NONE) }
        for (raw in listOf("", "storage", "VALIDATION", " STORAGE")) fails { DocumentDimension.parse(raw) }
        fails { DocumentState.parse(DocumentDimension.EXTRACTION, "STORED") }
    }

    @Test fun `replacement chain keeps both originals and is recorded once`() {
        val first = stored(201); val second = stored(202); val third = stored(203); val spare = stored(204)
        val (oldFirst, second2) = EnterpriseDocument.replace(first, second)
        assertEquals(DocumentState.REPLACED, oldFirst.statuses.storage); assertEquals(id(202), oldFirst.replacedBy)
        assertEquals(id(201), second2.replaces); assertEquals(first.digest, oldFirst.digest); assertEquals(second.digest, second2.digest)
        assertEquals(DocumentState.STORED, first.statuses.storage); assertNull(first.replacedBy); assertNull(second.replaces)
        fails(DocumentFailure.INVALID_TRANSITION) { EnterpriseDocument.replace(oldFirst, third) }
        fails(DocumentFailure.DUPLICATE) { EnterpriseDocument.replace(spare, second2) }
        val (oldSecond, third2) = EnterpriseDocument.replace(second2, third)
        assertEquals(id(201), oldSecond.replaces); assertEquals(id(203), oldSecond.replacedBy); assertEquals(id(202), third2.replaces)
        val deleted = oldFirst.go(DocumentDimension.STORAGE, DocumentState.REPLACED, DocumentState.DELETED)
        assertEquals(id(202), deleted.replacedBy); assertEquals(first.digest, deleted.digest)
        fails(DocumentFailure.SCOPE) { EnterpriseDocument.replace(first, EnterpriseDocument.create(other, id(205), digest(5))) }
    }

    @Test fun `allocations and splits reuse the existing line bound`() {
        var d = document().withAmount(1_000_000_000_000)
        val targets = (0 until 1001).map { expense(10000 + it) }
        for (target in targets) d = d.link(target)
        val lines = targets.map { DocumentAllocationLine(it, 1) }
        fails { d.allocate(lines) }
        fails { d.splitEvenly(targets) }
        val full = d.allocate(lines.take(1000))
        assertEquals(1000L, full.allocated); assertEquals(DocumentState.PARTIALLY_ALLOCATED, full.statuses.reconciliation)
        val split = d.splitEvenly(targets.take(1000))
        assertEquals(1_000_000_000_000L, split.allocated); assertEquals(DocumentState.RECONCILED, split.statuses.reconciliation)
    }

    @Test fun `fiscal consultation instants stay inside the shared profile`() {
        val d = document()
        for (bad in listOf(Instant.MAX, Instant.MIN, Instant.parse("+10000-01-01T00:00:00Z"), Instant.parse("0000-12-31T23:59:59Z"))) {
            fails { d.go(DocumentDimension.FISCAL, DocumentState.NOT_CONSULTED, DocumentState.UNAVAILABLE, bad) }
        }
        val minimum = Instant.parse("0001-01-01T00:00:00Z")
        assertEquals(minimum, d.go(DocumentDimension.FISCAL, DocumentState.NOT_CONSULTED, DocumentState.UNAVAILABLE, minimum).consultedAt)
    }

    @Test fun `collections are immutable copies and values are redacted`() {
        val a = expense(211); val b = expense(212); val c = expense(213)
        val one = document().withAmount(100).link(a)
        val two = one.link(b); val three = one.link(c)
        assertEquals(listOf(a, b), two.links); assertEquals(listOf(a, c), three.links); assertEquals(listOf(a), one.links)
        @Suppress("UNCHECKED_CAST")
        assertThrows(UnsupportedOperationException::class.java) { (two.links as MutableList<DocumentTarget>).add(c) }
        val input = mutableListOf(DocumentAllocationLine(a, 60), DocumentAllocationLine(b, 40))
        val allocated = two.allocate(input)
        input.clear()
        assertEquals(100L, allocated.allocated); assertEquals(2, allocated.allocations.size)
        @Suppress("UNCHECKED_CAST")
        assertThrows(UnsupportedOperationException::class.java) { (allocated.allocations as MutableList<DocumentAllocationLine>).clear() }
        assertEquals(expense(211), a); assertEquals(expense(211).hashCode(), a.hashCode()); assertNotEquals(expense(211, other), a)
        assertEquals(DocumentAllocationLine(a, 60).hashCode(), allocated.allocations.first().hashCode())
        assertEquals(setOf(DocumentAllocationLine(a, 60)), setOf(DocumentAllocationLine(expense(211), 60)))
        val secrets = listOf(id(201), id(211), org, digest(201), "100")
        for (text in listOf(allocated.toString(), a.toString(), allocated.allocations.first().toString())) {
            for (secret in secrets) assertFalse(secret in text)
        }
    }

    @Test fun `allocations never exceed their source and splits match the BR-MONEY-007 allocator`() {
        val random = Random(185189)
        val targets = (211..214).map { expense(it) }
        var base = document()
        for (target in targets) base = base.link(target)
        repeat(2000) {
            val amount = random.nextLong(200000)
            val d = base.withAmount(amount)
            val lines = targets.take(random.nextInt(targets.size + 1)).map { DocumentAllocationLine(it, 1 + random.nextLong(100000)) }
            val sum = lines.sumOf { it.centavos }
            if (sum > amount) {
                fails(DocumentFailure.EXCEEDS_AMOUNT) { d.allocate(lines) }
                assertEquals(0L, d.allocated)
                return@repeat
            }
            val after = d.allocate(lines)
            assertEquals(sum, after.allocated); assertEquals(amount - sum, after.remaining)
            assertEquals(expected(amount, sum), after.statuses.reconciliation)
            val lower = random.nextLong(amount + 1)
            val corrected = after.withAmount(lower)
            assertEquals(expected(lower, sum), corrected.statuses.reconciliation); assertEquals(after.allocations, corrected.allocations)
            val count = 1 + random.nextInt(targets.size)
            val split = d.splitEvenly(targets.take(count))
            val reference = EnterpriseValues.allocate(amount, (0 until count).map { AllocationLine(it.toLong(), 1) }).parts!!
            val want = reference.filter { it.centavos > 0 }.map { DocumentAllocationLine(targets[it.order.toInt()], it.centavos) }
            assertEquals(want, split.allocations); assertEquals(amount, split.allocated)
        }
    }

    private fun expected(amount: Long, sum: Long) = when {
        sum == 0L -> DocumentState.UNALLOCATED
        sum < amount -> DocumentState.PARTIALLY_ALLOCATED
        sum == amount -> DocumentState.RECONCILED
        else -> DocumentState.OVER_ALLOCATED
    }
}
