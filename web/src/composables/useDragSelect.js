import { ref, onMounted, onUnmounted } from 'vue'

/**
 * 拖选功能 - 在表格中按住鼠标拖动快速多选行
 * 用法：把返回的 rowProps 传给 NDataTable 的 :row-props
 *
 * @param {Ref<Array>} checkedKeys - 已选中的行 keys
 * @param {String} rowKeyField - 行 key 字段名，默认 'name'
 * @returns {Object} { rowProps, isDragging }
 */
export function useDragSelect(checkedKeys, rowKeyField = 'name') {
  const isDragging = ref(false)
  const initialCheckedState = ref(false)

  const toggleSelection = (key, shouldSelect) => {
    const index = checkedKeys.value.indexOf(key)
    if (shouldSelect && index === -1) {
      checkedKeys.value.push(key)
    } else if (!shouldSelect && index > -1) {
      checkedKeys.value.splice(index, 1)
    }
  }

  const handleMouseDown = (rowKey, e) => {
    if (rowKey === undefined || rowKey === null) return
    // 只在勾选框这一列启用拖选，避免误触
    const selectionCell = e.target.closest('.n-data-table-td--selection')
    if (!selectionCell) return

    // 标记进入拖选状态，但不主动改动当前行的选中状态，
    // 让 NaiveUI 自己处理这一次点击的勾选/取消，
    // 我们只对「经过的其他行」做统一批量处理，避免双重切换导致状态错乱。
    isDragging.value = true

    // 记录初始行的目标选中状态：
    // - 如果一开始是未选中，则拖动经过的行统一改为“选中”
    // - 如果一开始是已选中，则拖动经过的行统一改为“取消选中”
    initialCheckedState.value = !checkedKeys.value.includes(rowKey)
  }

  const handleMouseEnter = (rowKey) => {
    if (rowKey === undefined || rowKey === null) return
    if (!isDragging.value) return
    toggleSelection(rowKey, initialCheckedState.value)
  }

  const handleMouseUp = () => {
    if (!isDragging.value) return
    isDragging.value = false
  }

  onMounted(() => {
    document.addEventListener('mouseup', handleMouseUp)
  })

  onUnmounted(() => {
    document.removeEventListener('mouseup', handleMouseUp)
  })

  const rowProps = (row) => {
    const rowKey = row?.[rowKeyField]
    return {
      'data-key': rowKey,
      onMousedown: (e) => handleMouseDown(rowKey, e),
      onMouseenter: () => handleMouseEnter(rowKey)
    }
  }

  return {
    rowProps,
    isDragging
  }
}
