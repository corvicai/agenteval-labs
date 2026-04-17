/**
 * useCollabQuestionSet — composable for accessing all question sets
 * (own + shared) and determining access level for a given QS.
 */
import { computed } from 'vue'
import { useWSStore } from '../stores/wsStore.js'

export function useCollabQuestionSet() {
    const { state } = useWSStore()

    /**
     * All question sets: own ones first, then shared ones.
     * Each shared QS is decorated with { _shared: true, owner_user_id, owner_name, role }.
     */
    const allQuestionSets = computed(() => {
        const own = Array.isArray(state.questionSets) ? state.questionSets : []
        const shared = Array.isArray(state.sharedQuestionSets) ? state.sharedQuestionSets : []
        return [...own, ...shared.map(s => ({ ...s, _shared: true }))]
    })

    /**
     * Returns the access level for the given question set ID:
     *   'owner'  — owned by current user (in state.questionSets)
     *   'editor' — shared with role 'editor'
     *   'viewer' — shared with role 'viewer'
     *   'none'   — not found / no access
     */
    function getAccessLevel(qsId) {
        if (!qsId) return 'none'
        const own = Array.isArray(state.questionSets) ? state.questionSets : []
        if (own.some(q => q.id === qsId)) return 'owner'

        const shared = Array.isArray(state.sharedQuestionSets) ? state.sharedQuestionSets : []
        const sharedQS = shared.find(q => q.id === qsId)
        if (sharedQS) return sharedQS.role || 'editor'

        return 'none'
    }

    function isOwner(qsId) {
        return getAccessLevel(qsId) === 'owner'
    }

    function isEditor(qsId) {
        const level = getAccessLevel(qsId)
        return level === 'owner' || level === 'editor'
    }

    return {
        allQuestionSets,
        getAccessLevel,
        isOwner,
        isEditor
    }
}

export default useCollabQuestionSet
