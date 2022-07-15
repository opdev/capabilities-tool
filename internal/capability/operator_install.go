package capability

import (
	"context"
	"strings"

	log "opcap/internal/logger"
	"opcap/internal/operator"

	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
)

var logger = log.Sugar

// TODO: InstallOperatorsTest creates all subscriptions for a catalogSource sequencially
// We will need other arguments that can tweak how many to test at a time
// And possibly indicate a specific condition

type operatorData struct {
	namespace     string
	targetNs1     string
	targetNs2     string
	operatorGroup string
	installedNS   []string
}

type InstallModeTypeAllNamespaces struct {}

type InstallModeTypeOwnNamespace struct {}

type InstallModeTypeSingleNamespace struct {}

type InstallModeTypeMultiNamespace struct {}

type installModeData interface {
	createFromOpertorData(data operatorData) (operator.OperatorGroupData)
 }

func OperatorInstallAllFromCatalog(catalogSource string, catalogSourceNamespace string) error {
	s, err := operator.Subscriptions(catalogSource, catalogSourceNamespace)
	if err != nil {
		logger.Errorf("Error while getting bundles from CatalogSource %s: %w", catalogSource, err)
		return err
	}

	c, err := operator.NewClient()
	if err != nil {
		logger.Errorf("Error while creating PackageServerClient: %w", err)
		return err
	}

	for _, subscription := range s {

		// TODO: implement this with goroutines for concurrent testing
		// TODO: transform subscriptions list in a queuing mechanism
		// for the test work. Run all individual tests under the umbrella
		// of it's operator dedicated goroutine
		err := OperatorInstall(subscription, c)
		if err != nil {
			logger.Errorw("installing operator", "package", subscription.Package, "channel", subscription.Channel, "installmode", subscription.InstallModeType)
		}

	}

	return nil
}

func OperatorInstall(s operator.SubscriptionData, c operator.Client) error {
	logger.Debugw("installing package", "package", s.Package, "channel", s.Channel, "installmode", s.InstallModeType)

	od := new(operatorData)

	od.namespace = strings.Join([]string{"opcap", strings.ReplaceAll(s.Package, ".", "-")}, "-")
	od.targetNs1 = strings.Join([]string{od.namespace, "targetns1"}, "-")
	od.targetNs2 = strings.Join([]string{od.namespace, "targetns2"}, "-")
	od.operatorGroup = strings.Join([]string{s.Name, s.Channel, "group"}, "-")

	// create operator namespace
	operator.CreateNamespace(context.Background(), od.namespace)

	// Checking install modes and
	// creating operatorGroup per operator package/channel
	od.installedNS = []string{od.namespace}
	createGroupByInstallMode(s, c, *od)

	// create subscription per operator package/channel
	sub, err := c.CreateSubscription(context.Background(), s, od.namespace)
	if err != nil {
		logger.Debugf("Error creating subscriptions: %w", err)
		return err
	}

	if err = c.WaitForInstallPlan(context.Background(), sub); err != nil {
		logger.Debugf("Waiting for InstallPlan: %w", err)
		return err
	}
	// check/approve install plan
	// TODO: check the name standard for installPlan
	err = c.InstallPlanApprove(od.namespace)
	if err != nil {
		logger.Debugf("Error creating subscriptions: %w", err)
		return err
	}

	csvStatus, err := c.WaitForCsvOnNamespace(od.namespace)

	if err != nil {
		logger.Infow("failed", "package", s.Package, "channel", s.Channel, "installmode", s.InstallModeType)
	} else {
		logger.Infow(strings.ToLower(csvStatus), "package", s.Package, "channel", s.Channel, "installmode", s.InstallModeType)
	}

	cleanUp(s, c, *od)

	return nil
}

func createGroupByInstallMode(s operator.SubscriptionData, c operator.Client, m operatorData) {

	switch s.InstallModeType {

		case operatorv1alpha1.InstallModeTypeAllNamespaces:
	
			installModeData := &InstallModeTypeAllNamespaces{}
			opGroupData := installModeData.createFromOperatorData(m)
			c.CreateOperatorGroup(context.Background(), opGroupData, m.namespace)
	
		case operatorv1alpha1.InstallModeTypeMultiNamespace:
			installModeData := &InstallModeTypeMultiNamespace{}
			opGroupData := installModeData.createFromOperatorData(m)
			c.CreateOperatorGroup(context.Background(), opGroupData, m.namespace)
		
		case operatorv1alpha1.InstallModeTypeOwnNamespace:
	
			installModeData := &InstallModeTypeOwnNamespace{}
			opGroupData := installModeData.createFromOperatorData(m)
			c.CreateOperatorGroup(context.Background(), opGroupData, m.namespace)
		
		case operatorv1alpha1.InstallModeTypeSingleNamespace:
			installModeData := &InstallModeTypeSingleNamespace{}
			opGroupData := installModeData.createFromOperatorData(m)
			c.CreateOperatorGroup(context.Background(), opGroupData, m.namespace)
		}
}

func (m *InstallModeTypeAllNamespaces) createFromOperatorData(data operatorData) operator.OperatorGroupData{
	opGroupData := operator.OperatorGroupData{
	   Name:             data.operatorGroup,
	   TargetNamespaces: []string{},
	}
	return opGroupData
 }
 
 func (m *InstallModeTypeMultiNamespace) createFromOperatorData(data operatorData) (operator.OperatorGroupData){
	 operator.CreateNamespace(context.Background(), data.targetNs1)
	 operator.CreateNamespace(context.Background(), data.targetNs2)
	 opGroupData := operator.OperatorGroupData{
		 Name:			data.operatorGroup,
		 TargetNamespaces: []string{data.targetNs1, data.targetNs2},
	 }
	 return opGroupData
 }
 
 func (m *InstallModeTypeOwnNamespace) createFromOperatorData(data operatorData) (operator.OperatorGroupData){
	 opGroupData := operator.OperatorGroupData{
		 Name:			  data.operatorGroup,
		 TargetNamespaces: []string{data.namespace},
	 }
	 return opGroupData
 }
 
 func (m *InstallModeTypeSingleNamespace) createFromOperatorData(data operatorData) (operator.OperatorGroupData){
	 operator.CreateNamespace(context.Background(), data.targetNs1)
	 opGroupData := operator.OperatorGroupData{
		 Name:			  data.operatorGroup,
		 TargetNamespaces: []string{data.targetNs1},
	 }
	 return opGroupData
 }

func cleanUp(s operator.SubscriptionData, c operator.Client, m operatorData) {

	// delete subscription
	err := c.DeleteSubscription(context.Background(), s.Name, m.namespace)
	if err != nil {
		logger.Debugf("Error while deleting Subscription: %w", err)
		return
	}

	// delete operator group
	err = c.DeleteOperatorGroup(context.Background(), m.operatorGroup, m.namespace)
	if err != nil {
		logger.Debugf("Error while deleting OperatorGroup: %w", err)
		return
	}

	// delete namespaces
	for _, ns := range m.installedNS {
		operator.DeleteNamespace(context.Background(), ns)
	}
}