export interface OrganizerCapture {
	id: string;
	sourceType: string;
	rawText: string;
	status: 'received' | 'parsing' | 'needs_review' | 'confirmed' | 'failed';
	error?: string;
	createdAt: string;
}

export interface OrganizerItem {
	id: string;
	type: 'task' | 'event' | 'reminder';
	title: string;
	description?: string;
	startAt?: string;
	endAt?: string;
	dueAt?: string;
	reminderAt?: string;
	timezone: string;
	allDay: boolean;
	priority: number;
	status: 'inbox' | 'todo' | 'doing' | 'done' | 'cancelled';
	project?: string;
	tags?: string[];
	location?: string;
	version: number;
	createdAt: string;
	updatedAt: string;
}

export interface OrganizerItemsResponse { items: OrganizerItem[]; }
export interface OrganizerCapturesResponse { captures: OrganizerCapture[]; }

