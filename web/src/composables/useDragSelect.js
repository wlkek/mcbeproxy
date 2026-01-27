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
  const suppressNextSelectionClick = ref(false)

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
    const selectionCell = e.target.closest('.n-data-table-td--selection')
    if (!selectionCell) return

    isDragging.value = true
    suppressNextSelectionClick.value = true

    initialCheckedState.value = !checkedKeys.value.includes(rowKey)
    toggleSelection(rowKey, initialCheckedState.value)

    e.preventDefault()
    e.stopPropagation()
  }

  const handleMouseEnter = (rowKey) => {
    if (rowKey === undefined || rowKey === null) return
    if (!isDragging.value) return
    toggleSelection(rowKey, initialCheckedState.value)
  }

  const handleMouseUp = () => {
    if (!isDragging.value) return
    isDragging.value = false

    // click 事件会发生在 mouseup 之后，这里延迟清理，避免双切换
    setTimeout(() => {
      suppressNextSelectionClick.value = false
    }, 0)
  }

  const handleClickCapture = (e) => {
    if (!suppressNextSelectionClick.value) return

    const selectionCell = e.target.closest('.n-data-table-td--selection')
    if (!selectionCell) return

    suppressNextSelectionClick.value = false
    e.preventDefault()
    e.stopPropagation()
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
      onMouseenter: () => handleMouseEnter(rowKey),
      onClickCapture: handleClickCapture
    }
  }

  return {
    rowProps,
    isDragging
  }
}
