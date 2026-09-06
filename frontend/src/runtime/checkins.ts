type TaskLike = {
  type?: unknown;
  taskID?: unknown;
  part?: unknown;
  completed?: unknown;
  ownRecord?: unknown;
};

type CompletionLike = {
  user_id?: unknown;
  task_id?: unknown;
  task_type?: unknown;
  part?: unknown;
  completed?: unknown;
  inherited?: unknown;
  record?: unknown;
};

type MemberLike = {
  user_id?: unknown;
};

export function taskIsCompleted(task: TaskLike): boolean {
  return Boolean(task?.completed || task?.ownRecord);
}

export function completionMatchesTask(completion: CompletionLike, task: TaskLike): boolean {
  if (String(completion?.task_type || '') !== String(task?.type || '')) return false;
  const taskID = Number(task?.taskID || 0);
  if (taskID > 0) return Number(completion?.task_id || 0) === taskID;
  return String(completion?.part || '') === String(task?.part || '');
}

export function buildTaskCompletionMatrix(
  members: MemberLike[],
  tasks: TaskLike[],
  completions: CompletionLike[],
) {
  const byUser = new Map<number, Array<{
    task: TaskLike;
    completion: CompletionLike | null;
    record: unknown;
    done: boolean;
  }>>();
  let doneSlots = 0;

  for (const member of members) {
    const userID = Number(member?.user_id || 0);
    const userCompletions = completions.filter((item) => Number(item?.user_id || 0) === userID);
    const taskStates = tasks.map((task) => {
      const completion = userCompletions.find((item) => completionMatchesTask(item, task)) || null;
      const done = Boolean(completion?.completed);
      if (done) doneSlots += 1;
      return {
        task,
        completion,
        record: completion?.record || null,
        done,
      };
    });
    byUser.set(userID, taskStates);
  }

  return { byUser, doneSlots };
}
