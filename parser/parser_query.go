package parser

import (
	"errors"
	"fmt"
	"strings"
)

func (p *Parser) tryParseWithExpr(pos Pos) (*WithExpr, error) {
	if !p.matchKeyword(KeywordWith) {
		return nil, nil
	}
	return p.parseWithExpr(pos)
}

func (p *Parser) parseWithExpr(pos Pos) (*WithExpr, error) {
	if err := p.consumeKeyword(KeywordWith); err != nil {
		return nil, err
	}

	cteExpr, err := p.parseCTEExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	ctes := []*CTEExpr{cteExpr}
	for p.tryConsumeTokenKind(",") != nil {
		cteExpr, err := p.parseCTEExpr(p.Pos())
		if err != nil {
			return nil, err
		}
		ctes = append(ctes, cteExpr)
	}

	return &WithExpr{
		WithPos: pos,
		CTEs:    ctes,
		EndPos:  ctes[len(ctes)-1].End(),
	}, nil
}

func (p *Parser) tryParseTopExpr(pos Pos) (*TopExpr, error) {
	if !p.matchKeyword(KeywordTop) {
		return nil, nil
	}
	return p.parseTopExpr(pos)
}

func (p *Parser) parseTopExpr(pos Pos) (*TopExpr, error) {
	if err := p.consumeKeyword(KeywordTop); err != nil {
		return nil, err
	}

	number, err := p.parseNumber(p.Pos())
	if err != nil {
		return nil, err
	}
	topEnd := number.End()

	withTies := false
	if p.tryConsumeKeyword(KeywordWith) != nil {
		topEnd = p.last().End
		if err := p.consumeKeyword(KeywordTies); err != nil {
			return nil, err
		}
		withTies = true
	}
	return &TopExpr{
		TopPos:   pos,
		TopEnd:   topEnd,
		Number:   number,
		WithTies: withTies,
	}, nil
}

func (p *Parser) tryParseFromExpr(pos Pos) (*FromExpr, error) {
	if !p.matchKeyword(KeywordFrom) {
		return nil, nil
	}
	return p.parseFromExpr(pos)
}

func (p *Parser) parseFromExpr(pos Pos) (*FromExpr, error) {
	if err := p.consumeKeyword(KeywordFrom); err != nil {
		return nil, err
	}

	expr, err := p.parseJoinExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	return &FromExpr{
		FromPos: pos,
		Expr:    expr,
	}, nil
}

func (p *Parser) tryParseJoinConstraints(pos Pos) (Expr, error) {
	switch {
	case p.tryConsumeKeyword(KeywordOn) != nil:
		columnExprList, err := p.parseColumnExprListWithRoundBracket(p.Pos())
		if err != nil {
			return nil, err
		}
		return &OnExpr{
			OnPos: pos,
			On:    columnExprList,
		}, nil
	case p.tryConsumeKeyword(KeywordUsing) != nil:
		hasParen := p.tryConsumeTokenKind("(") != nil
		columnExprList, err := p.parseColumnExprListWithRoundBracket(p.Pos())
		if err != nil {
			return nil, err
		}
		if hasParen {
			if _, err := p.consumeTokenKind(")"); err != nil {
				return nil, err
			}
		}
		return &UsingExpr{
			UsingPos: pos,
			Using:    columnExprList,
		}, nil
	}
	return nil, nil
}

func (p *Parser) parseJoinOp(_ Pos) (Expr, error) {
	switch {
	case p.tryConsumeKeyword(KeywordCross) != nil: // cross join
		if err := p.consumeKeyword(KeywordJoin); err != nil {
			return nil, err
		}
	case p.tryConsumeTokenKind(",") != nil:
	case p.matchKeyword(KeywordAny), p.matchKeyword(KeywordAll):
		_ = p.lexer.consumeToken()
		if p.matchKeyword(KeywordFull) {
			_ = p.lexer.consumeToken()
		}
		if p.matchKeyword(KeywordLeft) || p.matchKeyword(KeywordRight) || p.matchKeyword(KeywordInner) || p.matchKeyword(KeywordOuter) {
			_ = p.lexer.consumeToken()
		}
	case p.matchKeyword(KeywordSemi), p.matchKeyword(KeywordAsof):
		_ = p.lexer.consumeToken()
		if p.matchKeyword(KeywordLeft) || p.matchKeyword(KeywordRight) {
			_ = p.lexer.consumeToken()
		}
		if p.matchKeyword(KeywordOuter) {
			_ = p.lexer.consumeToken()
		}
	case p.matchKeyword(KeywordInner):
		_ = p.lexer.consumeToken()
		if p.matchKeyword(KeywordAll) || p.matchKeyword(KeywordAny) || p.matchKeyword(KeywordAsof) {
			_ = p.lexer.consumeToken()
		}
	case p.matchKeyword(KeywordLeft), p.matchKeyword(KeywordRight):
		if p.matchKeyword(KeywordOuter) {
			_ = p.lexer.consumeToken()
		}
		if p.matchKeyword(KeywordSemi) || p.matchKeyword(KeywordAnti) ||
			p.matchKeyword(KeywordAny) || p.matchKeyword(KeywordAll) ||
			p.matchKeyword(KeywordAsof) || p.matchKeyword(KeywordArray) {
			_ = p.lexer.consumeToken()
		}
	case p.matchKeyword(KeywordFull):
		_ = p.lexer.consumeToken()
		if p.matchKeyword(KeywordOuter) {
			_ = p.lexer.consumeToken()
		}
		if p.matchKeyword(KeywordAll) || p.matchKeyword(KeywordAny) {
			_ = p.lexer.consumeToken()
		}
	default:
		return nil, nil
	}
	_ = p.tryConsumeKeyword(KeywordJoin)
	return p.parseJoinExpr(p.Pos())
}

func (p *Parser) parseJoinExpr(pos Pos) (expr Expr, err error) {
	var sampleRatio *SampleRatioExpr
	switch {
	case p.matchTokenKind(TokenIdent), p.matchTokenKind("("):
		expr, err = p.parseTableExpr(p.Pos())
		if err != nil {
			return nil, err
		}
		_ = p.tryConsumeKeyword(KeywordFinal)

		sampleRatio, err = p.tryParseSampleRationExpr(p.Pos())
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("expected table name or subquery, got %v", p.last())
	}

	// TODO: store global/local in AST
	if p.matchKeyword(KeywordGlobal) || p.matchKeyword(KeywordLocal) {
		_ = p.lexer.consumeToken()
	}
	rightExpr, err := p.parseJoinOp(p.Pos())
	if err != nil {
		return nil, err
	}
	// rightExpr is nil means no join op
	if rightExpr == nil {
		return
	}
	constrains, err := p.tryParseJoinConstraints(p.Pos())
	if err != nil {
		return nil, err
	}
	return &JoinExpr{
		JoinPos:     pos,
		Left:        expr,
		Right:       rightExpr,
		SampleRatio: sampleRatio,
		Constraints: constrains,
	}, nil
}

func (p *Parser) parseTableExpr(pos Pos) (*TableExpr, error) {
	var expr Expr
	var err error
	switch {
	case p.matchTokenKind(TokenIdent):
		// table name
		tableIdentifier, err := p.parseTableIdentifier(p.Pos())
		if err != nil {
			return nil, err
		}
		// it's a table name
		if tableIdentifier.Database != nil || !p.matchTokenKind("(") { // database.table
			expr = tableIdentifier
		} else {
			// table function expr
			tableArgs, err := p.parseTableArgList(p.Pos())
			if err != nil {
				return nil, err
			}
			expr = &TableFunctionExpr{
				Name: tableIdentifier.Table,
				Args: tableArgs,
			}
		}
	case p.matchTokenKind("("):
		expr, err = p.parseSelectUnionExprList(p.Pos())
	default:
		return nil, errors.New("expect table name or subquery")
	}
	if err != nil {
		return nil, err
	}

	tableEnd := expr.End()
	if asToken := p.tryConsumeKeyword(KeywordAs); asToken != nil {
		alias, err := p.parseIdent()
		if err != nil {
			return nil, err
		}
		expr = &AliasExpr{
			Expr:     expr,
			AliasPos: asToken.Pos,
			Alias:    alias,
		}
		tableEnd = expr.End()
	}
	return &TableExpr{
		TablePos: pos,
		TableEnd: tableEnd,
		Expr:     expr,
	}, nil
}

func (p *Parser) tryParsePrewhereExpr(pos Pos) (*PrewhereExpr, error) {
	if !p.matchKeyword(KeywordPrewhere) {
		return nil, nil
	}
	return p.parsePrewhereExpr(pos)
}
func (p *Parser) parsePrewhereExpr(pos Pos) (*PrewhereExpr, error) {
	if err := p.consumeKeyword(KeywordPrewhere); err != nil {
		return nil, err
	}

	expr, err := p.parseExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	return &PrewhereExpr{
		PrewherePos: pos,
		Expr:        expr,
	}, nil
}

func (p *Parser) tryParseWhereExpr(pos Pos) (*WhereExpr, error) {
	if !p.matchKeyword(KeywordWhere) {
		return nil, nil
	}
	return p.parseWhereExpr(pos)
}

func (p *Parser) parseWhereExpr(pos Pos) (*WhereExpr, error) {
	if err := p.consumeKeyword(KeywordWhere); err != nil {
		return nil, err
	}

	expr, err := p.parseExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	return &WhereExpr{
		WherePos: pos,
		Expr:     expr,
	}, nil
}

func (p *Parser) tryParseGroupByExpr(pos Pos) (*GroupByExpr, error) {
	if !p.matchKeyword(KeywordGroup) {
		return nil, nil
	}
	return p.parseGroupByExpr(pos)
}

// syntax: groupByClause? (WITH (CUBE | ROLLUP))? (WITH TOTALS)?
func (p *Parser) parseGroupByExpr(pos Pos) (*GroupByExpr, error) {
	if err := p.consumeKeyword(KeywordGroup); err != nil {
		return nil, err
	}
	if err := p.consumeKeyword(KeywordBy); err != nil {
		return nil, err
	}

	var expr Expr
	var err error
	aggregateType := ""
	if p.matchKeyword(KeywordCube) || p.matchKeyword(KeywordRollup) {
		aggregateType = p.last().String
		_ = p.lexer.consumeToken()
		expr, err = p.parseFunctionParams(p.Pos())
	} else {
		expr, err = p.parseColumnExprListWithRoundBracket(p.Pos())
	}
	if err != nil {
		return nil, err
	}

	groupByExpr := &GroupByExpr{
		GroupByPos:    pos,
		AggregateType: strings.ToUpper(aggregateType),
		Expr:          expr,
	}

	// parse WITH CUBE, ROLLUP, TOTALS
	for p.tryConsumeKeyword(KeywordWith) != nil {
		switch {
		case p.tryConsumeKeyword(KeywordCube) != nil:
			groupByExpr.WithCube = true
		case p.tryConsumeKeyword(KeywordRollup) != nil:
			groupByExpr.WithRollup = true
		case p.tryConsumeKeyword(KeywordTotals) != nil:
			groupByExpr.WithTotals = true
		default:
			return nil, fmt.Errorf("expected CUBE, ROLLUP or TOTALS, got %s", p.lastTokenKind())
		}
	}

	return groupByExpr, nil
}

func (p *Parser) tryParseLimitByExpr(pos Pos) (*LimitByExpr, error) {
	if !p.matchKeyword(KeywordLimit) {
		return nil, nil
	}
	return p.parseLimitByExpr(pos)
}

func (p *Parser) parseLimitByExpr(pos Pos) (*LimitByExpr, error) {
	if err := p.consumeKeyword(KeywordLimit); err != nil {
		return nil, err
	}

	limit, err := p.parseExpr(p.Pos())
	if err != nil {
		return nil, err
	}

	var offset Expr
	if p.tryConsumeKeyword(KeywordOffset) != nil {
		offset, err = p.parseExpr(p.Pos())
	} else if p.tryConsumeTokenKind(",") != nil {
		offset = limit
		limit, err = p.parseExpr(p.Pos())
	}
	if err != nil {
		return nil, err
	}

	var by *ColumnExprList
	if p.tryConsumeKeyword(KeywordBy) != nil {
		if by, err = p.parseColumnExprListWithRoundBracket(p.Pos()); err != nil {
			return nil, err
		}
	}
	return &LimitByExpr{
		LimitPos: pos,
		Limit:    limit,
		Offset:   offset,
		ByExpr:   by,
	}, nil
}

func (p *Parser) tryParseWindowFrameExpr(pos Pos) (*WindowFrameExpr, error) {
	if !p.matchKeyword(KeywordRows) && !p.matchKeyword(KeywordRange) {
		return nil, nil
	}
	return p.parseWindowFrameExpr(pos)
}

func (p *Parser) parseWindowFrameExpr(pos Pos) (*WindowFrameExpr, error) {
	var windowFrameType string
	if p.matchKeyword(KeywordRows) || p.matchKeyword(KeywordRange) {
		windowFrameType = strings.ToUpper(p.last().String)
		_ = p.lexer.consumeToken()
	}

	var expr Expr
	switch {
	case p.tryConsumeKeyword(KeywordBetween) != nil:
		betweenExpr, err := p.parseWindowFrameExpr(p.Pos())
		if err != nil {
			return nil, err
		}

		andPos := p.Pos()
		if err := p.consumeKeyword(KeywordAnd); err != nil {
			return nil, err
		}

		andExpr, err := p.parseWindowFrameExpr(p.Pos())
		if err != nil {
			return nil, err
		}
		expr = &WindowFrameRangeExpr{
			BetweenPos:  pos,
			BetweenExpr: betweenExpr,
			AndPos:      andPos,
			AndExpr:     andExpr,
		}
	case p.matchKeyword(KeywordCurrent):
		currentPos := p.Pos()
		_ = p.lexer.consumeToken()
		rowEnd := p.last().End
		if err := p.consumeKeyword(KeywordRow); err != nil {
			return nil, err
		}
		expr = &WindowFrameCurrentRow{
			CurrentPos: currentPos,
			RowEnd:     rowEnd,
		}
	case p.matchKeyword(KeywordUnbounded):
		unboundedPos := p.Pos()
		_ = p.lexer.consumeToken()

		direction := ""
		switch {
		case p.matchKeyword(KeywordPreceding), p.matchKeyword(KeywordFollowing):
			direction = p.last().String
			_ = p.lexer.consumeToken()
		default:
			return nil, fmt.Errorf("expected PRECEDING or FOLLOWING, got %s", p.lastTokenKind())
		}
		expr = &WindowFrameUnbounded{
			UnboundedPos: unboundedPos,
			Direction:    direction,
		}
	case p.matchTokenKind(TokenInt):
		number, err := p.parseNumber(p.Pos())
		if err != nil {
			return nil, err
		}

		var unboundedEnd Pos
		direction := ""
		switch {
		case p.matchKeyword(KeywordPreceding), p.matchKeyword(KeywordFollowing):
			direction = p.last().String
			unboundedEnd = p.last().End
			_ = p.lexer.consumeToken()
		default:
			return nil, fmt.Errorf("expected PRECEDING or FOLLOWING, got %s", p.lastTokenKind())
		}
		expr = &WindowFrameNumber{
			UnboundedEnd: unboundedEnd,
			Number:       number,
			Direction:    direction,
		}
	default:
		return nil, fmt.Errorf("expected BETWEEN, CURRENT, UNBOUNDED or integer, got %s", p.lastTokenKind())
	}
	return &WindowFrameExpr{
		FramePos: pos,
		Type:     windowFrameType,
		Extend:   expr,
	}, nil
}

func (p *Parser) tryParseWindowExpr(pos Pos) (*WindowExpr, error) {
	if !p.matchKeyword(KeywordWindow) {
		return nil, nil
	}
	return p.parseWindowExpr(pos)
}

func (p *Parser) parseWindowCondition(pos Pos) (*WindowConditionExpr, error) {
	if _, err := p.consumeTokenKind("("); err != nil {
		return nil, err
	}
	partitionBy, err := p.tryParsePartitionByExpr(pos)
	if err != nil {
		return nil, err
	}
	orderBy, err := p.tryParseOrderByExprList(p.Pos())
	if err != nil {
		return nil, err
	}
	frame, err := p.tryParseWindowFrameExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	rightParenPos := p.Pos()
	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}
	return &WindowConditionExpr{
		LeftParenPos:  pos,
		RightParenPos: rightParenPos,
		PartitionBy:   partitionBy,
		OrderBy:       orderBy,
		Frame:         frame,
	}, nil
}

func (p *Parser) parseWindowExpr(pos Pos) (*WindowExpr, error) {
	if err := p.consumeKeyword(KeywordWindow); err != nil {
		return nil, err
	}

	windowName, err := p.parseIdent()
	if err != nil {
		return nil, err
	}

	if err := p.consumeKeyword(KeywordAs); err != nil {
		return nil, err
	}

	condition, err := p.parseWindowCondition(p.Pos())
	if err != nil {
		return nil, err
	}

	return &WindowExpr{
		WindowPos:           pos,
		Name:                windowName,
		WindowConditionExpr: condition,
	}, nil
}

func (p *Parser) tryParseArrayJoin(pos Pos) (*ArrayJoinExpr, error) {
	if !p.matchKeyword(KeywordLeft) && !p.matchKeyword(KeywordInner) && !p.matchKeyword(KeywordArray) {
		return nil, nil
	}
	return p.parseArrayJoin(pos)
}

func (p *Parser) parseArrayJoin(_ Pos) (*ArrayJoinExpr, error) {
	var typ string
	switch {
	case p.matchKeyword(KeywordLeft), p.matchKeyword(KeywordInner):
		typ = p.last().String
		_ = p.lexer.consumeToken()
	}
	arrayPos := p.Pos()
	if err := p.consumeKeyword(KeywordArray); err != nil {
		return nil, err
	}

	if err := p.consumeKeyword(KeywordJoin); err != nil {
		return nil, err
	}

	expr, err := p.parseColumnExprList(p.Pos())
	if err != nil {
		return nil, err
	}

	return &ArrayJoinExpr{
		ArrayPos: arrayPos,
		Type:     typ,
		Expr:     expr,
	}, nil
}

func (p *Parser) tryParseHavingExpr(pos Pos) (*HavingExpr, error) {
	if !p.matchKeyword(KeywordHaving) {
		return nil, nil
	}
	return p.parseHavingExpr(pos)
}

func (p *Parser) parseHavingExpr(pos Pos) (*HavingExpr, error) {
	if err := p.consumeKeyword(KeywordHaving); err != nil {
		return nil, err
	}

	expr, err := p.parseColumnsExpr(p.Pos())
	if err != nil {
		return nil, err
	}

	return &HavingExpr{
		HavingPos: pos,
		Expr:      expr,
	}, nil
}

func (p *Parser) parseSubQuery(pos Pos) (*SubQueryExpr, error) {
	if err := p.consumeKeyword(KeywordAs); err != nil {
		return nil, err
	}

	selectExprList, err := p.parseSelectUnionExprList(p.Pos())
	if err != nil {
		return nil, err
	}

	return &SubQueryExpr{
		AsPos:   pos,
		Selects: selectExprList,
	}, nil
}

func (p *Parser) parseSelectUnionExprList(pos Pos) (*SelectExprList, error) {
	var selectQueries []*SelectExpr
	for {
		selectQuery, err := p.parseSelectQuery(pos)
		if err != nil {
			return nil, err
		}
		selectQueries = append(selectQueries, selectQuery)

		if p.tryConsumeKeyword(KeywordUnion) == nil {
			break
		}
		if err := p.consumeKeyword(KeywordAll); err != nil {
			return nil, err
		}
	}
	return &SelectExprList{
		Items: selectQueries,
	}, nil
}

func (p *Parser) parseSelectQuery(pos Pos) (*SelectExpr, error) {
	switch {
	case p.matchKeyword(KeywordSelect),
		p.matchKeyword(KeywordWith):
		return p.parseSelectStatement(pos)
	case p.matchTokenKind("("):
		return p.parseSelectStatementWithParen(pos)
	default:
		return nil, fmt.Errorf("expected SELECT, WITH or (, got %s", p.lastTokenKind())
	}
}

func (p *Parser) parseSelectStatementWithParen(_ Pos) (*SelectExpr, error) {
	if _, err := p.consumeTokenKind("("); err != nil {
		return nil, err
	}

	selectExpr, err := p.parseSelectStatement(p.Pos())
	if err != nil {
		return nil, err
	}

	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}

	return selectExpr, nil
}

func (p *Parser) parseSelectStatement(pos Pos) (*SelectExpr, error) {
	withExpr, err := p.tryParseWithExpr(pos)
	if err != nil {
		return nil, err
	}
	if err := p.consumeKeyword(KeywordSelect); err != nil {
		return nil, err
	}
	// DISTINCT?
	_ = p.tryConsumeKeyword(KeywordDistinct)

	topExpr, err := p.tryParseTopExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	selectColumns, err := p.parseColumnExprListWithRoundBracket(p.Pos())
	if err != nil {
		return nil, err
	}
	statementEnd := selectColumns.End()
	fromExpr, err := p.tryParseFromExpr(p.Pos())
	if err != nil {
		return nil, err
	}

	if fromExpr != nil {
		statementEnd = fromExpr.End()
	}
	arrayJoinExpr, err := p.tryParseArrayJoin(p.Pos())
	if err != nil {
		return nil, err
	}
	if arrayJoinExpr != nil {
		statementEnd = arrayJoinExpr.End()
	}
	windowExpr, err := p.tryParseWindowExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	if windowExpr != nil {
		statementEnd = windowExpr.End()
	}
	prewhereExpr, err := p.tryParsePrewhereExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	if prewhereExpr != nil {
		statementEnd = prewhereExpr.End()
	}
	whereExpr, err := p.tryParseWhereExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	if whereExpr != nil {
		statementEnd = whereExpr.End()
	}
	groupByExpr, err := p.tryParseGroupByExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	if groupByExpr != nil {
		statementEnd = groupByExpr.End()
	}
	havingExpr, err := p.tryParseHavingExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	if havingExpr != nil {
		statementEnd = havingExpr.End()
	}
	orderByExpr, err := p.tryParseOrderByExprList(p.Pos())
	if err != nil {
		return nil, err
	}
	if orderByExpr != nil {
		statementEnd = orderByExpr.End()
	}
	limitByExpr, err := p.tryParseLimitByExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	if limitByExpr != nil {
		statementEnd = limitByExpr.End()
	}
	settingsExpr, err := p.tryParseSettingsExprList(p.Pos())
	if err != nil {
		return nil, err
	}
	if settingsExpr != nil {
		statementEnd = settingsExpr.End()
	}

	return &SelectExpr{
		With:          withExpr,
		SelectPos:     pos,
		StatementEnd:  statementEnd,
		Top:           topExpr,
		SelectColumns: selectColumns,
		From:          fromExpr,
		ArrayJoin:     arrayJoinExpr,
		Window:        windowExpr,
		Prewhere:      prewhereExpr,
		Where:         whereExpr,
		GroupBy:       groupByExpr,
		Having:        havingExpr,
		OrderBy:       orderByExpr,
		LimitBy:       limitByExpr,
		Settings:      settingsExpr,
	}, nil
}

// ctes
//    : WITH namedQuery (',' namedQuery)*
//    ;

// namedQuery
//    : name=identifier (columnAliases)? AS '(' query ')'
//    ;

// columnAliases
//
//	: '(' identifier (',' identifier)* ')'
//	;
func (p *Parser) parseCTEExpr(pos Pos) (*CTEExpr, error) {
	name, err := p.parseIdent()
	if err != nil {
		return nil, err
	}
	columnAliases, err := p.tryParseColumnAliases()
	if err != nil {
		return nil, err
	}

	if err = p.consumeKeyword(KeywordAs); err != nil {
		return nil, err
	}

	if _, err := p.consumeTokenKind("("); err != nil {
		return nil, err
	}

	selectExpr, err := p.parseSelectStatement(p.Pos())
	if err != nil {
		return nil, err
	}

	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}

	return &CTEExpr{
		CTEPos:        pos,
		Name:          name,
		SelectExpr:    selectExpr,
		EndPos:        selectExpr.End(),
		ColumnAliases: columnAliases,
	}, nil
}

func (p *Parser) tryParseColumnAliases() ([]*Ident, error) {
	if !p.matchTokenKind("(") {
		return nil, nil
	}
	if _, err := p.consumeTokenKind("("); err != nil {
		return nil, err
	}

	aliasList := make([]*Ident, 0)
	for {
		ident, err := p.parseIdent()
		if err != nil {
			return nil, err
		}
		aliasList = append(aliasList, ident)
		if p.matchTokenKind(")") {
			break
		}
		if _, err := p.consumeTokenKind(","); err != nil {
			return nil, err
		}
	}
	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}
	return aliasList, nil
}

func (p *Parser) tryParseSampleRationExpr(pos Pos) (*SampleRatioExpr, error) {
	if !p.matchKeyword(KeywordSample) {
		return nil, nil
	}
	return p.parseSampleRationExpr(pos)
}

func (p *Parser) parseSampleRationExpr(pos Pos) (*SampleRatioExpr, error) {
	if err := p.consumeKeyword(KeywordSample); err != nil {
		return nil, err
	}
	ratio, err := p.parseFloat(p.Pos())
	if err != nil {
		return nil, err
	}

	var offset *FloatLiteral
	if p.matchKeyword(KeywordOffset) {
		_ = p.lexer.consumeToken()
		offset, err = p.parseFloat(p.Pos())
		if err != nil {
			return nil, err
		}
	}

	return &SampleRatioExpr{
		SamplePos: pos,
		Ratio:     ratio,
		Offset:    offset,
	}, nil
}