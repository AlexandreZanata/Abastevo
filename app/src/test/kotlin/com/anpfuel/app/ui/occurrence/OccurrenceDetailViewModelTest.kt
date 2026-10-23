package com.anpfuel.app.ui.occurrence

import com.anpfuel.domain.occurrence.*
import com.anpfuel.domain.repository.OccurrenceGateway
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.*
import org.junit.jupiter.api.Assertions.*

@OptIn(ExperimentalCoroutinesApi::class)
class OccurrenceDetailViewModelTest {
 private val dispatcher=StandardTestDispatcher()
 private val a="eeeeeeee-6666-4666-8666-000000000001"
 private val b="eeeeeeee-6666-4666-8666-000000000002"
 private val lease=OccurrenceLease(SourceRevision("enforcement","enforcement-v1","enforcement-source-v1",a),true,false)
 private fun row(id:String)=OccurrenceRecord(id,"enforcement-source:abc","company-shaped","DF","BRASILIA","Revenda","2026-10-04","document_day","DOC","Inspection","Synthetic","synthetic:2")
 @BeforeEach fun before(){Dispatchers.setMain(dispatcher)}
 @AfterEach fun after(){Dispatchers.resetMain()}
 private fun gateway(read:suspend (String)->Pair<OccurrenceRecord,OccurrenceLease>)=object:OccurrenceGateway{
  override suspend fun record(id:String,revisionId:String?)=read(id)
  override suspend fun records(uf:String?,municipality:String?,segment:String?,subjectKind:String?,fromDay:String?,toDay:String?,limit:Int,cursor:String?,revisionId:String?):OccurrencePage<OccurrenceRecord> = error("unused")
  override suspend fun recordCoverage(revisionId:String?):Pair<RecordCoverage,OccurrenceLease> = error("unused")
  override suspend fun stations(state:String?,segment:String?,fromDay:String?,toDay:String?,limit:Int,cursor:String?,revisionId:String?):OccurrencePage<OccurrenceStation> = error("unused")
  override suspend fun stationCoverage(revisionId:String?):Pair<StationCoverage,OccurrenceLease> = error("unused")
 }
 @Test fun `obsolete success cannot replace newer detail`()=runTest(dispatcher){
  val old=CompletableDeferred<Unit>()
  val vm=OccurrenceDetailViewModel(gateway{id->if(id==a)withContext(NonCancellable){old.await()}; row(id) to lease})
  vm.load(a);runCurrent();vm.load(b);runCurrent()
  assertEquals(b,vm.state.value.record?.id)
  old.complete(Unit);advanceUntilIdle()
  assertEquals(b,vm.state.value.record?.id)
  assertFalse(vm.state.value.unavailable)
 }
 @Test fun `obsolete failure cannot mark newer detail unavailable`()=runTest(dispatcher){
  val old=CompletableDeferred<Unit>()
  val vm=OccurrenceDetailViewModel(gateway{id->if(id==a)withContext(NonCancellable){old.await();throw java.io.IOException("down")}; row(id) to lease})
  vm.load(a);runCurrent();vm.load(b);runCurrent();old.complete(Unit);advanceUntilIdle()
  assertEquals(b,vm.state.value.record?.id)
  assertFalse(vm.state.value.unavailable)
 }
 @Test fun `new identity clears old detail and cached detail is marked stale`()=runTest(dispatcher){
  val wait=CompletableDeferred<Unit>()
  val vm=OccurrenceDetailViewModel(gateway{id->if(id==b)wait.await();row(id) to lease.copy(stale=true)})
  vm.load(a);advanceUntilIdle();assertTrue(vm.state.value.stale)
  vm.load(b);runCurrent();assertNull(vm.state.value.record)
  wait.complete(Unit);advanceUntilIdle();assertEquals(b,vm.state.value.record?.id)
 }
}
