package ai

import "github.com/yzxingtu2026/ainp/platform/modules/ai/memory"

type MemoryItem = memory.MemoryItem
type MemoryItemVersion = memory.MemoryItemVersion
type MemoryEvidence = memory.MemoryEvidence
type MemoryProjection = memory.MemoryProjection
type MemoryDaySummary = memory.MemoryDaySummary
type MemoryCurrentProfile = memory.MemoryCurrentProfile
type MemoryEvaluation = memory.MemoryEvaluation
type MemoryGovernanceDecision = memory.MemoryGovernanceDecision
type MemoryRelation = memory.MemoryRelation
type MemoryConsolidationBatch = memory.MemoryConsolidationBatch
type MemoryService = memory.MemoryService
type MemoryRepository = memory.MemoryRepository
type MemoryExtractionService = memory.MemoryExtractionService
type MemoryGovernanceService = memory.MemoryGovernanceService
type MemoryProjectionService = memory.MemoryProjectionService
type MemoryManagementService = memory.MemoryManagementService
type MemoryRetrievalService = memory.MemoryRetrievalService
type MemoryConsolidationService = memory.MemoryConsolidationService
type MemoryTool = memory.MemoryTool
type MemoryCandidateExtractor = memory.MemoryCandidateExtractor
type MemoryCreateRequest = memory.MemoryCreateRequest
type MemoryDecisionRequest = memory.MemoryDecisionRequest
type MemoryExtractionBacklog = memory.MemoryExtractionBacklog
type MemoryListRequest = memory.MemoryListRequest
type MemoryGetRequest = memory.MemoryGetRequest
type MemoryMutationRequest = memory.MemoryMutationRequest
type MemoryCorrectRequest = memory.MemoryCorrectRequest
type MemoryClearRequest = memory.MemoryClearRequest
type MemoryItemView = memory.MemoryItemView
type MemoryListResult = memory.MemoryListResult
type MemoryRecallRequest = memory.MemoryRecallRequest
type MemoryRecallResponse = memory.MemoryRecallResponse
type MemoryRecallItem = memory.MemoryRecallItem
type MemoryProvenanceRef = memory.MemoryProvenanceRef

var NewMemoryService = memory.NewMemoryService
var NewMemoryRepository = memory.NewMemoryRepository
var NewMemoryExtractionService = memory.NewMemoryExtractionService
var NewMemoryGovernanceService = memory.NewMemoryGovernanceService
var NewMemoryProjectionService = memory.NewMemoryProjectionService
var NewMemoryManagementService = memory.NewMemoryManagementService
var NewMemoryRetrievalService = memory.NewMemoryRetrievalService
var NewMemoryConsolidationService = memory.NewMemoryConsolidationService
var NewMemoryTool = memory.NewMemoryTool
var RegisterMemoryExtraction = memory.RegisterMemoryExtraction
var RegisterMemoryGovernance = memory.RegisterMemoryGovernance
var RegisterMemoryManagement = memory.RegisterMemoryManagement
var ValidateMemoryCreateRequest = memory.ValidateMemoryCreateRequest

const MemoryExtractionJobType = memory.MemoryExtractionJobType
const MemoryExtractionPromptVersion = memory.MemoryExtractionPromptVersion
const MemoryExtractionSchemaVersion = memory.MemoryExtractionSchemaVersion
const MemoryConsolidationJobType = memory.MemoryConsolidationJobType
const MemoryItemChangedEvent = memory.MemoryItemChangedEvent
const MemoryProjectionJobType = memory.MemoryProjectionJobType
const ActionMemoryExtractionRecover = memory.ActionMemoryExtractionRecover
const ActionMemoryList = memory.ActionMemoryList
const ActionMemoryGet = memory.ActionMemoryGet
const ActionMemoryProfile = memory.ActionMemoryProfile
const ActionMemoryConfirm = memory.ActionMemoryConfirm
const ActionMemoryReject = memory.ActionMemoryReject
const ActionMemoryCorrect = memory.ActionMemoryCorrect
const ActionMemoryDelete = memory.ActionMemoryDelete
const ActionMemoryClear = memory.ActionMemoryClear
