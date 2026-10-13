package com.anpfuel.data.local.preferences
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
class TrainingTermsSourceTest {
 @Test fun `full canonical bytes are available offline and hash identically`() {
  val bytes=javaClass.classLoader!!.getResourceAsStream(TrainingTermsSource.FILE)!!.readBytes()
  val terms=TrainingTermsSource.decode(bytes)
  assertEquals("training-terms-v1",terms.version)
  assertEquals(EXPECTED,terms.digest)
  assertTrue(terms.text.contains("67.852.667/0001-93"));assertTrue(terms.text.contains("DESATIVADA"))
  assertTrue(terms.text.endsWith("\n"));assertTrue(terms.text.contains("8. Segurança e alterações"))
 }
 @Test fun `invalid and oversized document cannot grant authority`() {
  for(bytes in listOf(byteArrayOf(-1),ByteArray(65537),byteArrayOf())) assertThrows(IllegalArgumentException::class.java) { TrainingTermsSource.decode(bytes) }
 }
 companion object { const val EXPECTED="4a9763192620d267311f49fd43c9f4502144fa3bf78edc04b4112982e87bf8ce" }
}
